package srvmgr

import (
	"context"
	"sync"
)

type Process interface {
	Died() <-chan struct{}
	IsAlive() bool
	Die()
	Dying() <-chan struct{}
	Deceased()
}

type life struct {
	running     context.Context
	stop        context.CancelFunc
	stopping    chan struct{}
	deceaseOnce sync.Once
}

// We could have an external context as the seed of the process context, BUT the process cannot die automatically
// when the external ctx is done. The process must enter in "Dying mode" and wait the `Deceased` method be called, so the
// process may finish up whatever it has to do before die. As a consequence, we may need to have a Go inner routine, and
// it does not pay off.

// Why not return the process Context?
// Answer: If we do, whoever watches that context will assume that the process is dead as soon as the context is done, and
// it is not necessarily true. The internal context is only used internally to let the process knows that it's time to stop.

func NewProcess() Process {
	ctx, stop := context.WithCancel(context.Background())
	return &life{
		running:     ctx,
		stop:        stop,
		stopping:    make(chan struct{}),
		deceaseOnce: sync.Once{},
	}
}

func (l *life) Died() <-chan struct{} {
	return l.running.Done()
}

func (l *life) IsAlive() bool {
	return l.running.Err() == nil
}

func (l *life) Die() {
	l.stop()
}

func (l *life) Dying() <-chan struct{} {
	return l.stopping
}

func (l *life) Deceased() {
	l.deceaseOnce.Do(func() {
		close(l.stopping)
	})
}
