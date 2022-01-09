package service

import "context"

type Task interface {
	Name() string
	Start() error
	Stop(ctx context.Context) error
}
