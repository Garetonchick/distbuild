//go:build !solution

package scheduler

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/api"
	"github.com/Garetonchick/distbuild/pkg/build"
)

var TimeAfter = time.After

type PendingJob struct {
	Job      *api.JobSpec
	Finished chan struct{}
	Result   *api.JobResult
}

type Config struct {
	CacheTimeout time.Duration
	DepsTimeout  time.Duration
}

type Scheduler struct {
	mu       sync.Mutex
	notEmpty *sync.Cond

	jobQ        []*PendingJob
	artifactLoc map[build.ID]api.WorkerID
	pending     map[build.ID]*PendingJob

	l *zap.SugaredLogger
}

func NewScheduler(l *zap.Logger, config Config) *Scheduler {
	sched := &Scheduler{
		artifactLoc: make(map[build.ID]api.WorkerID),
		pending:     make(map[build.ID]*PendingJob),
		l:           l.Sugar(),
	}
	sched.notEmpty = sync.NewCond(&sched.mu)
	return sched
}

func (c *Scheduler) LocateArtifact(id build.ID) (api.WorkerID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	wID, ok := c.artifactLoc[id]
	return wID, ok
}

func (c *Scheduler) OnJobComplete(workerID api.WorkerID, jobID build.ID, res *api.JobResult) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if res.Error != nil {
		return false
	}
	_, ok := c.artifactLoc[jobID]
	if ok {
		return false
	}
	c.artifactLoc[jobID] = workerID
	pendingJob, ok := c.pending[jobID]
	if !ok {
		return true
	}
	pendingJob.Result = res
	close(pendingJob.Finished)
	return true
}

func (c *Scheduler) ScheduleJob(job *api.JobSpec) *PendingJob {
	c.mu.Lock()
	defer c.mu.Unlock()

	if pendingJob, ok := c.pending[job.ID]; ok {
		return pendingJob
	}

	pendingJob := PendingJob{
		Job:      job,
		Finished: make(chan struct{}),
		Result:   &api.JobResult{},
	}
	c.jobQ = append(c.jobQ, &pendingJob)
	c.pending[job.ID] = &pendingJob
	c.notEmpty.Signal()

	return &pendingJob
}

func (c *Scheduler) PickJob(ctx context.Context, workerID api.WorkerID) *PendingJob {
	c.mu.Lock()
	defer c.mu.Unlock()

	waitNotEmpty := func(done chan struct{}, cancel *bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		for len(c.jobQ) == 0 && !*cancel {
			c.notEmpty.Wait()
		}
		done <- struct{}{}
	}

	done := make(chan struct{}, 1)

	for len(c.jobQ) == 0 {
		cancel := false
		c.mu.Unlock()
		go waitNotEmpty(done, &cancel)
		select {
		case <-done:
		case <-ctx.Done():
			c.mu.Lock()
			cancel = true
			c.notEmpty.Broadcast()
			return nil
		}
		c.mu.Lock()
	}

	job := c.jobQ[0]
	c.jobQ = c.jobQ[1:]
	return job
}

func (c *Scheduler) Stop() {
	panic("implement me")
}
