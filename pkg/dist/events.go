package dist

import (
	"context"
	"fmt"
	"sync"

	"github.com/Garetonchick/distbuild/pkg/build"
)

type BuildEvent struct {
	sigs map[build.ID]chan struct{}

	mu sync.Mutex
}

func NewBuildEvent() *BuildEvent {
	return &BuildEvent{
		sigs: make(map[build.ID]chan struct{}),
	}
}

func (e *BuildEvent) Signal(id build.ID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch, ok := e.sigs[id]
	if !ok {
		return fmt.Errorf("unknown id=%v", id)
	}
	close(ch)
	return nil
}

// must happen-before BuildEvent.Signal and BuildEvent.Wait
func (e *BuildEvent) Prepare(id build.ID) {
	ch := make(chan struct{})
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sigs[id] = ch
}

func (e *BuildEvent) Wait(ctx context.Context, id build.ID) error {
	ch := func() chan struct{} {
		e.mu.Lock()
		defer e.mu.Unlock()
		ch, ok := e.sigs[id]
		if !ok {
			return nil
		}
		return ch
	}()

	if ch == nil {
		return fmt.Errorf("unknown id=%v", id)
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
