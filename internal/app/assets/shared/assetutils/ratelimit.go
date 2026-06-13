package assetutils

import (
	"fmt"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
)

type rateLimitLogger interface {
	Verbose(a ...any)
	Warn(a ...any)
}

type RateLimitBackoff struct {
	quiesce *Quiesce
	pause   time.Duration
	logger  rateLimitLogger
	label   string
	max     int
	hits    int
}

func NewRateLimitBackoff(quiesce *Quiesce, pause time.Duration, logger rateLimitLogger, label string) *RateLimitBackoff {
	return &RateLimitBackoff{
		quiesce: quiesce,
		pause:   pause,
		logger:  logger,
		label:   label,
		max:     config.GetInt("max_rate_limit_waits"),
	}
}

func (b *RateLimitBackoff) Wait() bool {
	b.hits++
	b.quiesce.Extend(b.pause)
	if b.hits == 1 {
		b.logger.Verbose(fmt.Sprintf(">> rate limited %s; waiting it out", b.label))
	}
	if b.hits >= b.max {
		b.logger.Warn(fmt.Sprintf(">> still rate limited %s after %d waits; giving up on this one", b.label, b.hits))
		return false
	}
	return true
}
