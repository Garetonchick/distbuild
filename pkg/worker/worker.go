//go:build !solution

package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/api"
	"github.com/Garetonchick/distbuild/pkg/artifact"
	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/filecache"
)

type Worker struct {
	id              api.WorkerID
	fileCache       *filecache.Cache
	artifacts       *artifact.Cache
	heartbeatClient *api.HeartbeatClient
	filecacheClient *filecache.Client
	artifactHandler *artifact.Handler
	mux             *http.ServeMux
	workDir         string
	maxJobs         int

	l *zap.SugaredLogger
}

func New(
	workerID api.WorkerID,
	coordinatorEndpoint string,
	log *zap.Logger,
	fileCache *filecache.Cache,
	artifacts *artifact.Cache,
) *Worker {
	worker := &Worker{
		id:              workerID,
		fileCache:       fileCache,
		artifacts:       artifacts,
		heartbeatClient: api.NewHeartbeatClient(log, coordinatorEndpoint),
		filecacheClient: filecache.NewClient(log, coordinatorEndpoint),
		artifactHandler: artifact.NewHandler(log, artifacts),
		mux:             http.NewServeMux(),
		workDir:         path.Join(os.TempDir(), "worker-"+build.NewID().String()),
		maxJobs:         2,

		l: log.Sugar(),
	}
	worker.artifactHandler.Register(worker.mux)
	return worker
}

func (w *Worker) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.mux.ServeHTTP(rw, r)
}

func (w *Worker) Run(ctx context.Context) error {
	var runningJobIds []build.ID
	var runningJobs []api.JobSpec
	var finishedJobs []api.JobResult
	var addedArtifacts []build.ID

	for {
		heartbeatReq := api.HeartbeatRequest{
			WorkerID:       w.id,
			RunningJobs:    runningJobIds,
			FreeSlots:      w.maxJobs - len(runningJobs),
			FinishedJob:    finishedJobs,
			AddedArtifacts: addedArtifacts,
		}
		heartbeatResp, err := w.heartbeatClient.Heartbeat(ctx, &heartbeatReq)
		if err != nil {
			return err
		}

		for jobID, jobSpec := range heartbeatResp.JobsToRun {
			runningJobIds = append(runningJobIds, jobID)
			runningJobs = append(runningJobs, jobSpec)
		}

		finishedJobs = finishedJobs[:0]
		addedArtifacts = addedArtifacts[:0]

		if len(runningJobs) != 0 {
			jobSpec := runningJobs[0]
			runningJobs = runningJobs[1:]
			runningJobIds = runningJobIds[1:]
			jobRes, err := w.runJob(ctx, &jobSpec)
			if err != nil {
				w.l.Debugf("run job failed: %v", err)
				return err
			}
			finishedJobs = append(finishedJobs, *jobRes)
			addedArtifacts = append(addedArtifacts, jobRes.ID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (w *Worker) runJob(ctx context.Context, job *api.JobSpec) (*api.JobResult, error) {
	err := w.downloadMissingFiles(ctx, job.SourceFiles)
	if err != nil {
		return nil, err
	}
	err = w.downloadMissingArtifacts(ctx, job.Artifacts)
	if err != nil {
		return nil, err
	}

	sourceDir, closeSource, err := w.createSourceDir(job.ID, job.SourceFiles)
	if err != nil {
		return nil, err
	}
	defer closeSource()

	deps, closeArtifacts, err := w.openArtifacts(job.Deps)
	if err != nil {
		return nil, err
	}
	w.l.Debug("loaded artifacts")
	defer closeArtifacts()

	jobDir := path.Join(w.workDir, job.ID.String())
	// outputDir := path.Join(jobDir, "out")
	// err = os.MkdirAll(outputDir, 0777)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(jobDir)
	}()

	artifactPath, commit, abort, err := w.artifacts.Create(job.ID)
	if err != nil {
		return nil, err
	}
	commited := false
	defer func() {
		if !commited {
			_ = abort()
		}
	}()

	jobRes := api.JobResult{ID: job.ID}
	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	for _, cmd := range job.Cmds {
		rendered, err := cmd.Render(build.JobContext{
			SourceDir: sourceDir,
			OutputDir: artifactPath,
			Deps:      deps,
		})
		if err != nil {
			return nil, err
		}
		excode, err := execRenderedCmd(ctx, rendered, stdout, stderr)
		if err != nil {
			return nil, err
		}
		if excode != 0 {
			jobRes.ExitCode = excode
			serr := fmt.Sprintf("exit failure, exitcode = %d", excode)
			jobRes.Error = &serr
			break
		}
	}

	jobRes.Stdout = stdout.Bytes()
	jobRes.Stderr = stderr.Bytes()

	_ = commit()
	commited = true

	// artifactPath, commit, abort, err := w.artifacts.Create(job.ID)
	// commited := false
	// if err != nil {
	// 	return nil, err
	// }
	// defer func() {
	// 	if !commited {
	// 		abort()
	// 	}
	// }()

	// err = os.Rename(outputDir, artifactPath)
	// if err != nil {
	// 	return nil, err
	// }
	// commit()
	// commited = true

	return &jobRes, nil
}

func execNormalCmd(
	ctx context.Context,
	cmd *build.Cmd,
	stdout io.Writer,
	stderr io.Writer,
) (excode int, err error) {
	execCmd := exec.CommandContext(ctx, cmd.Exec[0], cmd.Exec[1:]...)
	execCmd.Env = cmd.Environ
	execCmd.Dir = cmd.WorkingDirectory
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	err = execCmd.Run()

	if err, ok := err.(*exec.ExitError); ok {
		return err.ExitCode(), nil
	}
	return 0, err
}

func execCatCmd(cmd *build.Cmd) error {
	return os.WriteFile(cmd.CatOutput, []byte(cmd.CatTemplate), 0666)
}

func execRenderedCmd(
	ctx context.Context,
	cmd *build.Cmd,
	stdout io.Writer,
	stderr io.Writer,
) (excode int, err error) {
	if len(cmd.Exec) != 0 {
		excode, err = execNormalCmd(ctx, cmd, stdout, stderr)
		if err != nil {
			return excode, err
		}
	}
	if cmd.CatOutput != "" {
		err := execCatCmd(cmd)
		if err != nil {
			return 0, err
		}
	}
	return 0, nil
}

func (w *Worker) openArtifacts(artifacts []build.ID) (paths map[build.ID]string, close func(), err error) {
	paths = make(map[build.ID]string)
	unlocks := make([]func(), 0, len(artifacts))

	close = func() {
		for _, unlock := range unlocks {
			unlock()
		}
	}

	for _, artifactID := range artifacts {
		path, unlock, err := w.artifacts.Get(artifactID)
		w.l.Debugf("error while opening artifact: %v", err)
		if err != nil {
			close()
			return nil, func() {}, err
		}
		paths[artifactID] = path
		unlocks = append(unlocks, unlock)
	}

	return paths, close, nil
}

func (w *Worker) createSourceDir(
	jobID build.ID,
	sourceFiles map[build.ID]string,
) (dir string, close func(), err error) {
	unlocks := make([]func(), 0, len(sourceFiles))
	dir = path.Join(w.workDir, jobID.String(), "src")

	close = func() {
		for _, unlock := range unlocks {
			unlock()
		}
		os.RemoveAll(dir)
	}

	for fileID, relPath := range sourceFiles {
		filedir := path.Join(dir, path.Dir(relPath))
		err = os.MkdirAll(filedir, 0777)
		if err != nil {
			close()
			return "", func() {}, err
		}
		w.l.Debugf("worker create dir: %s", filedir)
		cachePath, unlock, err := w.fileCache.Get(fileID)
		if err != nil {
			close()
			return "", func() {}, err
		}
		unlocks = append(unlocks, unlock)
		err = os.Link(cachePath, path.Join(dir, relPath))
		if err != nil {
			close()
			return "", func() {}, err
		}
	}
	return dir, close, nil
}

func (w *Worker) downloadMissingFiles(ctx context.Context, sourceFiles map[build.ID]string) error {
	for fileID := range sourceFiles {
		_, unlock, err := w.fileCache.Get(fileID)
		if err == nil {
			unlock()
			continue
		}
		err = w.filecacheClient.Download(ctx, w.fileCache, fileID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) downloadMissingArtifacts(ctx context.Context, artifacts map[build.ID]api.WorkerID) error {
	for artifactID, workerID := range artifacts {
		_, unlock, err := w.artifacts.Get(artifactID)
		if err == nil {
			unlock()
			continue
		}
		err = artifact.Download(ctx, workerID.String(), w.artifacts, artifactID)
		if err != nil {
			w.l.Debugf("failed to download artifact id=%v", artifactID.String())
			return err
		}
	}
	return nil
}
