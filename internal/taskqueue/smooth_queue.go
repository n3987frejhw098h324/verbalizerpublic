package taskqueue

import (
	"container/list"
	"sync"
	"time"
)

type UniformPacer struct {
	mu   sync.Mutex
	next time.Time
	gap  time.Duration
}

func NewUniformPacer(minGap time.Duration) *UniformPacer {
	if minGap < time.Millisecond {
		minGap = time.Millisecond
	}
	return &UniformPacer{gap: minGap}
}

func (p *UniformPacer) Wait() {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.IsZero() || slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.gap)
	p.mu.Unlock()

	if d := time.Until(slot); d > 0 {
		time.Sleep(d)
	}
}

func (p *UniformPacer) AddChill(d time.Duration) {
	if d <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.next.IsZero() {
		p.next = time.Now().Add(d)
		return
	}
	p.next = p.next.Add(d)
}

func (p *UniformPacer) Decrement() {}

type SmoothQueue[R any] struct {
	Limiter *UniformPacer

	sem                chan struct{}
	mutex              sync.Mutex
	tasks              *list.List
	isSchedulerRunning bool
}

func NewSmoothQueue[R any](maxConcurrent, startsPerMinute int) *SmoothQueue[R] {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if startsPerMinute < 1 {
		startsPerMinute = 1
	}
	gap := time.Minute / time.Duration(startsPerMinute)
	p := NewUniformPacer(gap)
	return &SmoothQueue[R]{
		Limiter: p,
		sem:     make(chan struct{}, maxConcurrent),
		tasks:   list.New(),
	}
}

func (q *SmoothQueue[R]) QueueTask(f func() (R, error)) chan TaskResult[R] {
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

func (q *SmoothQueue[R]) Chill(d time.Duration) {
	if q.Limiter != nil {
		q.Limiter.AddChill(d)
	}
}

func (q *SmoothQueue[R]) scheduler() {
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
	}
}
