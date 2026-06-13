package taskqueue

import (
	"container/list"
	"sync"
	"time"
)

type TaskResult[R any] struct {
	Result R
	Error  error
}

type task[R any] struct {
	Func func() (R, error)
	Chan chan TaskResult[R]
}

type Queue[R any] struct {
	Limiter *fixedWindow

	interval           time.Duration
	isSchedulerRunning bool
	mutex              sync.Mutex
	tasks              *list.List
	sem                chan struct{}
}

const defaultMaxConcurrent = 512

func New[R any](window time.Duration, limit int, maxConcurrent ...int) *Queue[R] {
	concurrency := min(limit, defaultMaxConcurrent)
	if len(maxConcurrent) > 0 && maxConcurrent[0] > 0 {
		concurrency = maxConcurrent[0]
	}
	if concurrency < 1 {
		concurrency = 1
	}

	return &Queue[R]{
		Limiter: newFixedWindow(window, limit),

		interval: window / time.Duration(limit),
		tasks:    list.New(),
		sem:      make(chan struct{}, concurrency),
	}
}

func (q *Queue[R]) QueueTask(f func() (R, error)) chan TaskResult[R] {
	c := make(chan TaskResult[R])

	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.tasks.PushBack(task[R]{
		Func: f,
		Chan: c,
	})

	if !q.isSchedulerRunning {
		q.isSchedulerRunning = true
		go q.scheduler()
	}

	return c
}

func (q *Queue[R]) scheduler() {
	for {
		q.mutex.Lock()
		if q.tasks.Len() == 0 {
			q.isSchedulerRunning = false
			q.mutex.Unlock()
			return
		}

		e := q.tasks.Front()
		t := e.Value.(task[R])

		q.tasks.Remove(e)
		q.mutex.Unlock()

		q.sem <- struct{}{}
		q.Limiter.Wait()
		go func(t task[R]) {
			defer func() { <-q.sem }()
			res, err := t.Func()
			t.Chan <- TaskResult[R]{
				Result: res,
				Error:  err,
			}
		}(t)

		time.Sleep(q.interval)
	}
}
