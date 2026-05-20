package codegraph

import (
	"sync"
	"testing"
	"time"

	"repobridge/internal/source"
)

func TestSchedulerStartsBackgroundIndex(t *testing.T) {
	var mu sync.Mutex
	var calls []source.Outcome
	scheduler := NewScheduler(SchedulerOptions{
		IndexFunc: func(outcome source.Outcome) error {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, outcome)
			return nil
		},
	})

	outcome := source.Outcome{Path: t.TempDir()}
	scheduler.Schedule(outcome)
	scheduler.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Path != outcome.Path {
		t.Fatalf("call = %#v, want scheduled outcome", calls[0])
	}
}

func TestSchedulerDoesNotBlockWhenQueueIsFull(t *testing.T) {
	block := make(chan struct{})
	scheduler := NewScheduler(SchedulerOptions{
		WorkerLimit: 1,
		QueueSize:   1,
		IndexFunc: func(source.Outcome) error {
			<-block
			return nil
		},
	})

	scheduler.Schedule(source.Outcome{Path: t.TempDir()})
	scheduler.Schedule(source.Outcome{Path: t.TempDir()})

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		for i := 0; i < 32; i++ {
			scheduler.Schedule(source.Outcome{Path: t.TempDir()})
		}
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Schedule blocked when queue was full")
	}

	close(block)
	scheduler.Wait()
}

func TestSchedulerIgnoresEmptyPath(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		IndexFunc: func(source.Outcome) error {
			t.Fatal("IndexFunc called for empty path")
			return nil
		},
	})

	scheduler.Schedule(source.Outcome{})
	scheduler.Wait()
}
