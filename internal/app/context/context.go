package context

import (
	"sync"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/checkpoint"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/clientpool"
)

type Context struct {
	Client          *roblox.Client
	Clients         *clientpool.Pool
	Logger          *logger
	PauseController *pauseController
	Response        *response.Response
	Checkpoint      *checkpoint.Store

	cancelOnce sync.Once
	cancelled  chan struct{}
}

func New(pool *clientpool.Pool, resp *response.Response) *Context {
	progress := console.NewProgress(func() console.Snapshot {
		p := resp.Progress()
		return console.Snapshot{
			Total:      p.Total,
			Processed:  p.Processed,
			Current:    p.Current,
			EtaSeconds: p.EtaSeconds,
		}
	})

	return &Context{
		Client:          pool.Primary(),
		Clients:         pool,
		Logger:          newLogger(progress),
		PauseController: newPauseController(),
		Response:        resp,
		cancelled:       make(chan struct{}),
	}
}

func (c *Context) Cancel() bool {
	triggered := false
	c.cancelOnce.Do(func() {
		close(c.cancelled)
		triggered = true
	})
	return triggered
}

func (c *Context) Cancelled() bool {
	select {
	case <-c.cancelled:
		return true
	default:
		return false
	}
}

func (c *Context) StartProgress() { c.Logger.progress.Start() }

func (c *Context) StopProgress() { c.Logger.progress.Stop() }
