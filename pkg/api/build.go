package api

import (
	"context"

	"github.com/Garetonchick/distbuild/pkg/build"
	"go.uber.org/zap"
)

type BuildRequest struct {
	Graph build.Graph
}

type BuildStarted struct {
	ID           build.ID
	MissingFiles []build.ID
}

type StatusUpdate struct {
	JobFinished   *JobResult
	BuildFailed   *BuildFailed
	BuildFinished *BuildFinished
}

type BuildFailed struct {
	Error string
}

type BuildFinished struct {
}

type UploadDone struct{}

type SignalRequest struct {
	UploadDone *UploadDone
}

type SignalResponse struct {
}

type StatusWriter interface {
	Started(rsp *BuildStarted) error
	Updated(update *StatusUpdate) error
}

type Service interface {
	StartBuild(ctx context.Context, request *BuildRequest, w StatusWriter) error
	SignalBuild(ctx context.Context, buildID build.ID, signal *SignalRequest) (*SignalResponse, error)
}

type StatusReader interface {
	Close() error
	Next() (*StatusUpdate, error)
}

const ErrorHeader = "Error-Header"

func logHelper(l *zap.SugaredLogger, err *error, serviceName string, funName string) (logEnd func()) {
	l.Debugf("start: %s %s", serviceName, funName)
	logEnd = func() {
		if *err != nil {
			l.Debugf("%s %s error: %v", serviceName, funName, *err)
		}
		if err := recover(); err != nil {
			l.Debugf("%s %s panic: %v", serviceName, funName, err)
			panic(err)
		}
		l.Debugf("finish: %s %s", serviceName, funName)
	}
	return logEnd
}
