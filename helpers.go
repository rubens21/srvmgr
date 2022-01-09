package service

import (
	"context"
	defaultErr "errors"
	"github.com/pkg/errors"
	"net"
)

type GrpcServer interface {
	Serve(lis net.Listener) error
	Stop()
}

func GrpcServerAsTask(name string, srv GrpcServer, lis net.Listener) Task {
	start := func() error {
		err := srv.Serve(lis)
		return errors.Wrap(err, "error closing grpc server")
	}

	stop := func(ctx context.Context) error {
		srv.Stop()
		err := lis.Close()
		if err != nil && !defaultErr.Is(err, net.ErrClosed) {
			return errors.Wrap(err, "error stopping listener")
		}
		return nil
	}

	return MakeTask(name, start, stop)
}
