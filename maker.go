package srvmgr

import (
	"context"
)

func MakeTask(name string, start func() error, stop func(ctx context.Context) error) Task {
	return &genericTask{
		name:  name,
		start: start,
		stop:  stop,
	}
}

type genericTask struct {
	name  string
	start func() error
	stop  func(ctx context.Context) error
}

func (g *genericTask) Name() string {
	return g.name
}

func (g *genericTask) StartWithContext(opts StartOptions) error {
	if opts.Ready != nil {
		opts.Ready.Done()
	}
	return g.start()
}

func (g *genericTask) Stop(ctx context.Context) error {
	return g.stop(ctx)
}

func DefaultStopWithProcess(ctx context.Context, process Process) error {
	process.Die()
	select {
	case <-ctx.Done():
	case <-process.Dying():
	}
	return nil
}
