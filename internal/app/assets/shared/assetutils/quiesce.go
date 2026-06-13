package assetutils

import (
	"sync"
	"time"
)

type Quiesce struct {
	mu    sync.Mutex
	until time.Time
}

func NewQuiesce() *Quiesce {
	return &Quiesce{}
}

func (q *Quiesce) Extend(d time.Duration) {
	if d <= 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if t := time.Now().Add(d); t.After(q.until) {
		q.until = t
	}
}

func (q *Quiesce) Wait() {
	q.mu.Lock()
	u := q.until
	q.mu.Unlock()
	if d := time.Until(u); d > 0 {
		time.Sleep(d)
	}
}
