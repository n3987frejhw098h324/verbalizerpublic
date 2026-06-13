package clientpool

import (
	"sync"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

const evictAfterConsecutiveFailures = 6

type Pool struct {
	all []*roblox.Client

	mu      sync.Mutex
	active  []*roblox.Client
	fails   map[*roblox.Client]int
	evicted map[*roblox.Client]bool
	next    uint64
}

func New(clients []*roblox.Client) *Pool {
	if len(clients) == 0 {
		panic("clientpool.New: at least one client is required")
	}
	all := append([]*roblox.Client(nil), clients...)
	return &Pool{
		all:     all,
		active:  append([]*roblox.Client(nil), all...),
		fails:   make(map[*roblox.Client]int),
		evicted: make(map[*roblox.Client]bool),
	}
}

func (p *Pool) Next() *roblox.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.active)
	if n == 0 {
		return p.all[0]
	}
	c := p.active[p.next%uint64(n)]
	p.next++
	return c
}

func (p *Pool) Primary() *roblox.Client {
	return p.all[0]
}

func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.active)
}

func (p *Pool) All() []*roblox.Client {
	return p.all
}

func (p *Pool) ReportSuccess(c *roblox.Client) {
	p.mu.Lock()
	delete(p.fails, c)
	p.mu.Unlock()
}

func (p *Pool) ReportFailure(c *roblox.Client) (evicted bool, remaining int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.evicted[c] {
		return false, len(p.active)
	}

	p.fails[c]++
	if p.fails[c] < evictAfterConsecutiveFailures || len(p.active) <= 1 {
		return false, len(p.active)
	}

	p.evicted[c] = true
	for i, ac := range p.active {
		if ac == c {
			p.active = append(p.active[:i], p.active[i+1:]...)
			break
		}
	}
	return true, len(p.active)
}
