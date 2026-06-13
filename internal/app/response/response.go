package response

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type ResponseItem struct {
	OldID int64 `json:"oldId"`
	NewID int64 `json:"newId"`
}
type Progress struct {
	Total          int     `json:"total"`
	Processed      int     `json:"processed"`
	Succeeded      int     `json:"succeeded"`
	Failed         int     `json:"failed"`
	Current        string  `json:"current"`
	Percent        float64 `json:"percent"`
	ElapsedSeconds float64 `json:"elapsedSeconds"`
	EtaSeconds     float64 `json:"etaSeconds"`
	StopReason     string  `json:"stopReason,omitempty"`
}

type Response struct {
	cache       []ResponseItem
	all         []ResponseItem
	mutex       sync.RWMutex
	onItemAdded func(i ResponseItem)

	progressMu sync.Mutex
	total      int
	processed  *atomic.Int32
	startTime  time.Time
	current    string

	succeeded atomic.Int32
	failed    atomic.Int32

	stopReason atomic.Value
	moderated  atomic.Bool

	skipMu        sync.Mutex
	skipLines     []string
	skipUnapplied int
}

func New(onItemAdded ...func(i ResponseItem)) *Response {
	var callback func(i ResponseItem)
	if len(onItemAdded) > 0 {
		callback = onItemAdded[0]
	}

	return &Response{
		cache:       make([]ResponseItem, 0),
		onItemAdded: callback,
	}
}

func (r *Response) AddItem(i ResponseItem) {
	r.succeeded.Add(1)

	r.mutex.Lock()
	r.cache = append(r.cache, i)
	r.all = append(r.all, i)
	cb := r.onItemAdded
	r.mutex.Unlock()

	if cb != nil {
		cb(i)
	}
}

func (r *Response) AddFailed(n int) {
	if n > 0 {
		r.failed.Add(int32(n))
	}
}

func (r *Response) SetStopped(reason string) {
	r.stopReason.Store(reason)
}

func (r *Response) SetModerated() {
	r.moderated.Store(true)
}

func (r *Response) Moderated() bool {
	return r.moderated.Load()
}

func (r *Response) All() []ResponseItem {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return append([]ResponseItem(nil), r.all...)
}

func (r *Response) SetSkipSummary(lines []string, unapplied int) {
	r.skipMu.Lock()
	r.skipLines = append([]string(nil), lines...)
	r.skipUnapplied = unapplied
	r.skipMu.Unlock()
}

func (r *Response) SkipLines() []string {
	r.skipMu.Lock()
	defer r.skipMu.Unlock()
	return append([]string(nil), r.skipLines...)
}

func (r *Response) SkipUnapplied() int {
	r.skipMu.Lock()
	defer r.skipMu.Unlock()
	return r.skipUnapplied
}

func (r *Response) stoppedReason() string {
	if v, ok := r.stopReason.Load().(string); ok {
		return v
	}
	return ""
}

func (r *Response) Clear() {
	r.mutex.Lock()
	r.cache = make([]ResponseItem, 0)
	r.mutex.Unlock()
}

func (r *Response) EncodeJSON(e *json.Encoder) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return e.Encode(r.cache)
}

func (r *Response) Len() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.cache)
}

func (r *Response) SetProgress(total int, processed *atomic.Int32) {
	r.progressMu.Lock()
	r.total = total
	r.processed = processed
	r.startTime = time.Now()
	r.current = ""
	r.succeeded.Store(0)
	r.failed.Store(0)
	r.stopReason.Store("")
	r.moderated.Store(false)
	r.progressMu.Unlock()

	r.mutex.Lock()
	r.all = nil
	r.mutex.Unlock()

	r.skipMu.Lock()
	r.skipLines = nil
	r.skipUnapplied = 0
	r.skipMu.Unlock()
}

func (r *Response) SetCurrent(name string) {
	r.progressMu.Lock()
	r.current = name
	r.progressMu.Unlock()
}

func (r *Response) Progress() Progress {
	r.progressMu.Lock()
	total := r.total
	start := r.startTime
	counter := r.processed
	current := r.current
	r.progressMu.Unlock()

	processed := 0
	if counter != nil {
		processed = int(counter.Load())
	}

	p := Progress{
		Total:      total,
		Processed:  processed,
		Succeeded:  int(r.succeeded.Load()),
		Failed:     int(r.failed.Load()),
		Current:    current,
		StopReason: r.stoppedReason(),
	}
	if start.IsZero() {
		return p
	}

	elapsed := time.Since(start).Seconds()
	p.ElapsedSeconds = elapsed
	if total > 0 {
		p.Percent = float64(processed) / float64(total) * 100
	}
	if processed > 0 && processed < total && elapsed > 0 {
		rate := float64(processed) / elapsed
		p.EtaSeconds = float64(total-processed) / rate
	}
	return p
}
