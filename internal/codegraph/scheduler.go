package codegraph

import (
	"strings"
	"sync"

	"repobridge/internal/source"
)

type SchedulerOptions struct {
	WorkerLimit int
	QueueSize   int
	IndexFunc   func(source.Outcome) error
}

type Scheduler struct {
	queue     chan source.Outcome
	indexFunc func(source.Outcome) error
	wg        sync.WaitGroup
}

func NewScheduler(opts SchedulerOptions) *Scheduler {
	workers := opts.WorkerLimit
	if workers <= 0 {
		workers = 2
	}
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = workers * 4
	}
	indexFunc := opts.IndexFunc
	if indexFunc == nil {
		indexFunc = func(outcome source.Outcome) error {
			_, err := NewIndexer(IndexOptions{}).Index(outcome.Path)
			return err
		}
	}

	scheduler := &Scheduler{
		queue:     make(chan source.Outcome, queueSize),
		indexFunc: indexFunc,
	}
	for i := 0; i < workers; i++ {
		go scheduler.work()
	}
	return scheduler
}

func (s *Scheduler) Schedule(outcome source.Outcome) {
	if s == nil || strings.TrimSpace(outcome.Path) == "" {
		return
	}

	s.wg.Add(1)
	select {
	case s.queue <- outcome:
	default:
		s.wg.Done()
	}
}

func (s *Scheduler) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}

func (s *Scheduler) work() {
	for outcome := range s.queue {
		func() {
			defer s.wg.Done()
			defer func() {
				_ = recover()
			}()
			_ = s.indexFunc(outcome)
		}()
	}
}
