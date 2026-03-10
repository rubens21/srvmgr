package srvmgr

import "context"

type StartDone interface {
	Done()
}

type StartOptions struct {
	Ready StartDone
}

type Task interface {
	Name() string
	Start() error
	StartWithContext(opts StartOptions) error
	Stop(ctx context.Context) error
}

type Logger interface {
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}
