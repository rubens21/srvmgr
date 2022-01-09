package srvmgr

import "context"

type Task interface {
	Name() string
	Start() error
	Stop(ctx context.Context) error
}

type Logger interface {
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}
