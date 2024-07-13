//go:build !solution

package dist

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/api"
	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/filecache"
	"github.com/Garetonchick/distbuild/pkg/scheduler"
)

type Coordinator struct {
	filecache *filecache.Cache
	scheduler *scheduler.Scheduler

	filecacheHandler *filecache.Handler
	buildHandler     *api.BuildHandler
	heartbeatHandler *api.HeartbeatHandler

	buildEvent *BuildEvent

	mux *http.ServeMux
	l   *zap.SugaredLogger
}

var defaultConfig = scheduler.Config{
	CacheTimeout: time.Millisecond * 10,
	DepsTimeout:  time.Millisecond * 100,
}

func NewCoordinator(
	log *zap.Logger,
	fileCache *filecache.Cache,
) *Coordinator {
	c := &Coordinator{
		filecache: fileCache,
		scheduler: scheduler.NewScheduler(log, defaultConfig),

		filecacheHandler: filecache.NewHandler(log, fileCache),

		buildEvent: NewBuildEvent(),

		mux: http.NewServeMux(),

		l: log.Sugar(),
	}
	c.buildHandler = api.NewBuildService(log, c)
	c.heartbeatHandler = api.NewHeartbeatHandler(log, c)

	c.filecacheHandler.Register(c.mux)
	c.buildHandler.Register(c.mux)
	c.heartbeatHandler.Register(c.mux)
	return c
}

func (c *Coordinator) Stop() {}

func (c *Coordinator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}

func (c *Coordinator) StartBuild(
	ctx context.Context,
	request *api.BuildRequest,
	w api.StatusWriter,
) error {
	buildID := build.NewID()
	missingFiles := c.findMissingFiles(request.Graph.SourceFiles)

	c.buildEvent.Prepare(buildID)
	_ = w.Started(&api.BuildStarted{ID: buildID, MissingFiles: missingFiles})

	err := c.buildEvent.Wait(ctx, buildID)
	if err != nil {
		_ = w.Updated(&api.StatusUpdate{BuildFailed: &api.BuildFailed{Error: err.Error()}})
		return err
	}

	err = c.processGraph(ctx, request.Graph, w)
	if err != nil {
		_ = w.Updated(&api.StatusUpdate{BuildFailed: &api.BuildFailed{Error: err.Error()}})
		return err
	}

	_ = w.Updated(&api.StatusUpdate{BuildFinished: &api.BuildFinished{}})
	return nil
}

func (c *Coordinator) SignalBuild(
	ctx context.Context,
	buildID build.ID,
	signal *api.SignalRequest,
) (*api.SignalResponse, error) {
	_, _ = ctx, signal
	err := c.buildEvent.Signal(buildID)
	return &api.SignalResponse{}, err
}

func (c *Coordinator) Heartbeat(ctx context.Context, req *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {
	for _, jobRes := range req.FinishedJob {
		c.l.Debugf("got job result %v", jobRes)
		c.scheduler.OnJobComplete(req.WorkerID, jobRes.ID, &jobRes)
	}

	if req.FreeSlots <= 0 {
		return &api.HeartbeatResponse{JobsToRun: make(map[build.ID]api.JobSpec)}, nil
	}
	pendingJob := c.scheduler.PickJob(ctx, req.WorkerID)
	if pendingJob == nil {
		return &api.HeartbeatResponse{JobsToRun: make(map[build.ID]api.JobSpec)}, nil
	}
	jobsToRun := make(map[build.ID]api.JobSpec)
	jobsToRun[pendingJob.Job.ID] = *pendingJob.Job

	return &api.HeartbeatResponse{JobsToRun: jobsToRun}, nil
}

func (c *Coordinator) findMissingFiles(needFiles map[build.ID]string) []build.ID {
	var missingFiles []build.ID

	for id := range needFiles {
		_, unlock, err := c.filecache.Get(id)
		if err != nil {
			missingFiles = append(missingFiles, id)
		} else {
			unlock()
		}
	}
	return missingFiles
}

func (c *Coordinator) processGraph(ctx context.Context, g build.Graph, w api.StatusWriter) error {
	path2fileID := make(map[string]build.ID)

	for fileID, path := range g.SourceFiles {
		path2fileID[path] = fileID
	}

	convertSourceFiles := func(paths []string) map[build.ID]string {
		sourceFiles := make(map[build.ID]string)
		for _, path := range paths {
			sourceFiles[path2fileID[path]] = path
		}
		return sourceFiles
	}

	getArtifactLocations := func(artifacts []build.ID) (map[build.ID]api.WorkerID, error) {
		artifactLocs := make(map[build.ID]api.WorkerID)
		for _, artifactID := range artifacts {
			worker, ok := c.scheduler.LocateArtifact(artifactID)
			if !ok {
				return nil, errors.New("artifact not found")
			}
			artifactLocs[artifactID] = worker
		}
		return artifactLocs, nil
	}

	graphDist := NewGraphDistributer(&g)
	// var pendingJobs []*scheduler.PendingJob

	// TODO: add context
	err := graphDist.DistributeGraph(func(job *build.Job) error {
		artifacts, err := getArtifactLocations(job.Deps)
		if err != nil {
			return err
		}
		pendingJob := c.scheduler.ScheduleJob(&api.JobSpec{
			SourceFiles: convertSourceFiles(job.Inputs),
			Artifacts:   artifacts,
			Job:         *job,
		})
		// pendingJobs = append(pendingJobs, pendingJob)
		select {
		case <-pendingJob.Finished:
		case <-ctx.Done():
			return ctx.Err()
		}

		c.l.Debugf("got pending job result: %v", pendingJob.Result)
		_ = w.Updated(&api.StatusUpdate{JobFinished: pendingJob.Result})
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
