//go:build !solution

package client

import (
	"context"
	"errors"
	"path"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/api"
	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/filecache"
)

type Client struct {
	buildClient  *api.BuildClient
	uploadClient *filecache.Client
	sourceDir    string

	l *zap.SugaredLogger
}

func NewClient(
	l *zap.Logger,
	apiEndpoint string,
	sourceDir string,
) *Client {
	return &Client{
		buildClient:  api.NewBuildClient(l, apiEndpoint),
		uploadClient: filecache.NewClient(l, apiEndpoint),
		sourceDir:    sourceDir,

		l: l.Sugar(),
	}
}

type BuildListener interface {
	OnJobStdout(jobID build.ID, stdout []byte) error
	OnJobStderr(jobID build.ID, stderr []byte) error

	OnJobFinished(jobID build.ID) error
	OnJobFailed(jobID build.ID, code int, error string) error
}

func (c *Client) Build(ctx context.Context, graph build.Graph, lsn BuildListener) error {
	// start build
	buildStarted, statusReader, err := c.buildClient.StartBuild(ctx, &api.BuildRequest{Graph: graph})
	if err != nil {
		return err
	}

	// upload missing files
	for _, fileID := range buildStarted.MissingFiles {
		err = c.uploadClient.Upload(ctx, fileID, path.Join(c.sourceDir, graph.SourceFiles[fileID]))
		if err != nil {
			c.l.Debugf("failed to upload missing files, err: %v", err)
			return err
		}
	}
	c.l.Debugf("finish uploading missing files", err)

	// finished uploads, notify coordinator
	_, err = c.buildClient.SignalBuild(ctx, buildStarted.ID, &api.SignalRequest{})
	if err != nil {
		return err
	}

	// status updates
	defer statusReader.Close()
	for {
		update, err := statusReader.Next()
		if err != nil {
			return err
		}
		if update.BuildFailed != nil {
			return errors.New(update.BuildFailed.Error)
		}
		if update.JobFinished != nil {
			res := update.JobFinished
			if res.Error != nil {
				err = lsn.OnJobFailed(res.ID, res.ExitCode, *res.Error)
				if err != nil {
					return err
				}
				return errors.New(*res.Error)
			}
			if len(res.Stdout) != 0 {
				err = lsn.OnJobStdout(res.ID, res.Stdout)
				if err != nil {
					return err
				}
			}
			if len(res.Stderr) != 0 {
				err = lsn.OnJobStderr(res.ID, res.Stderr)
				if err != nil {
					return err
				}
			}
			err = lsn.OnJobFinished(res.ID)
			if err != nil {
				return err
			}
		}
		if update.BuildFinished != nil {
			break
		}
	}
	return nil
}
