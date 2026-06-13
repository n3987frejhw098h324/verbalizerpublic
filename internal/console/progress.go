package console

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
)

type Snapshot struct {
	Total      int
	Processed  int
	Current    string
	EtaSeconds float64
}
type Progress struct {
	snapshot func() Snapshot

	mu      sync.Mutex
	started bool
	visible bool
	done    chan struct{}
}

const (
	clearLine = "\r\x1b[K"
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"

	barWidth = 20
	maxName  = 24
)

func NewProgress(snapshot func() Snapshot) *Progress {
	return &Progress{snapshot: snapshot}
}

func (p *Progress) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		if _, err := color.Output.Write(b); err != nil {
			return 0, err
		}
		return len(b), nil
	}

	if p.visible {
		fmt.Fprint(color.Output, clearLine)
		p.visible = false
	}
	if _, err := color.Output.Write(b); err != nil {
		return 0, err
	}
	p.draw()
	return len(b), nil
}

func (p *Progress) Start() {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.done = make(chan struct{})
	done := p.done
	p.draw()
	p.mu.Unlock()

	go p.loop(done)
}

func (p *Progress) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started {
		return
	}
	p.started = false
	close(p.done)
	if p.visible {
		fmt.Fprint(color.Output, clearLine)
		p.visible = false
	}
}

func (p *Progress) loop(done chan struct{}) {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.started {
				p.draw()
			}
			p.mu.Unlock()
		}
	}
}

func (p *Progress) draw() {
	s := p.snapshot()

	filled := 0
	percent := 0
	if s.Total > 0 {
		filled = s.Processed * barWidth / s.Total
		if filled > barWidth {
			filled = barWidth
		}
		percent = s.Processed * 100 / s.Total
		if percent > 100 {
			percent = 100
		}
	}

	bar := ansiGreen + strings.Repeat("#", filled) + ansiReset + strings.Repeat(".", barWidth-filled)

	line := fmt.Sprintf("%s[%s] %3d%%  %d/%d", clearLine, bar, percent, s.Processed, s.Total)
	if s.EtaSeconds > 0 {
		line += "  ETA " + formatETA(s.EtaSeconds)
	}
	if s.Current != "" {
		line += "  " + ansiCyan + ">> " + truncate(s.Current, maxName) + ansiReset
	}

	fmt.Fprint(color.Output, line)
	p.visible = true
}

func formatETA(sec float64) string {
	d := time.Duration(sec) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[:n])
	}
	return string(r[:n-3]) + "..."
}
