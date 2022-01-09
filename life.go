package srvmgr

import (
	"context"
)

type Process interface {
	Live() <-chan struct{}
	IsAlive() bool
	Die()
	Dying() <-chan struct{}
	IsDeceased()
}

type life struct {
	running  context.Context
	stop     context.CancelFunc
	stopping chan struct{}
}

func NewProcess() Process {
	ctx, stop := context.WithCancel(context.Background())
	return &life{
		running:  ctx,
		stop:     stop,
		stopping: make(chan struct{}),
	}
}

func (l *life) Live() <-chan struct{} {
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

func (l *life) IsDeceased() {
	close(l.stopping)
}
