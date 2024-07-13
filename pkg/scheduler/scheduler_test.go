package scheduler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Garetonchick/distbuild/pkg/api"
	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/scheduler"
	"go.uber.org/zap/zaptest"
)

func Produce(s *scheduler.Scheduler, id build.ID, jobName string) *scheduler.PendingJob {
	job := api.JobSpec{
		Job: build.Job{
			Name: jobName,
			ID:   id,
		},
	}
	return s.ScheduleJob(&job)
}

func TestOneJob(t *testing.T) {
	s := scheduler.NewScheduler(zaptest.NewLogger(t), scheduler.Config{})
	workerID := api.WorkerID("w1")

	schedPending := Produce(s, build.NewID(), "job1")
	pickedPending := s.PickJob(context.Background(), workerID)
	s.OnJobComplete(workerID, schedPending.Job.ID, &api.JobResult{Stdout: []byte("kek")})

	<-pickedPending.Finished
	require.Equal(t, pickedPending.Result.Stdout, []byte("kek"))
}

func TestPendingJob(t *testing.T) {
	s := scheduler.NewScheduler(zaptest.NewLogger(t), scheduler.Config{})

	schedPending1 := Produce(s, build.NewID(), "job1")
	schedPending2 := Produce(s, schedPending1.Job.ID, "job1 again")
	assert.Equal(t, schedPending1.Job.Name, schedPending2.Job.Name)
	assert.Equal(t, "job1", schedPending2.Job.Name)
	assert.True(t, schedPending1 == schedPending2) // must have the same adress
}
