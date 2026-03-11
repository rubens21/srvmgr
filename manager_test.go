package srvmgr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManagerRunFailsWhenAnyTaskFailsToStart(t *testing.T) {
	t.Parallel()

	failErr := errors.New("failed to start")
	manager := NewManager(testLogger{}, time.Second, nil)
	manager.AddTask(
		&failingStartTask{name: "bad", err: failErr},
		newBlockingTask("good"),
	)

	err := manager.Run()
	if !errors.Is(err, failErr) {
		t.Fatalf("expected startup error %v, got %v", failErr, err)
	}
}

func TestManagerRunFailsWhenTaskFailsBeforeAnotherStarts(t *testing.T) {
	t.Parallel()

	failErr := errors.New("failed before others started")
	manager := NewManager(testLogger{}, time.Second, nil)
	manager.AddTask(
		&failingStartTask{name: "bad", err: failErr},
		newDelayedStartTask("slow"),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.Run()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, failErr) {
			t.Fatalf("expected startup error %v, got %v", failErr, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manager.Run() did not fail when task failed before another started")
	}
}

func TestManagerRunCallsCallbackAfterAllServicesAreUp(t *testing.T) {
	t.Parallel()

	upCalled := make(chan struct{}, 1)
	manager := NewManager(testLogger{}, time.Second, func() {
		upCalled <- struct{}{}
	})
	manager.AddTask(
		newBlockingTask("one"),
		newBlockingTask("two"),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.Run()
	}()

	select {
	case <-upCalled:
		_ = manager.Shutdown(context.Background())
	case <-time.After(2 * time.Second):
		t.Fatal("all-up callback was not executed")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected manager to stop without error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manager.Run() did not return after shutdown")
	}
}

func TestManagerRunDoesNotCallCallbackWhenStartFails(t *testing.T) {
	t.Parallel()

	upCalled := make(chan struct{}, 1)
	failErr := errors.New("startup failure")
	manager := NewManager(testLogger{}, time.Second, func() {
		upCalled <- struct{}{}
	})
	manager.AddTask(
		&failingStartTask{name: "bad", err: failErr},
		newDelayedStartTask("slow"),
	)

	err := manager.Run()
	if !errors.Is(err, failErr) {
		t.Fatalf("expected startup error %v, got %v", failErr, err)
	}
	select {
	case <-upCalled:
		t.Fatal("all-up callback should not execute on startup failure")
	default:
	}
}

type failingStartTask struct {
	name string
	err  error
}

func (t *failingStartTask) Name() string {
	return t.name
}

func (t *failingStartTask) StartWithContext(opts StartOptions) error {
	return t.err
}

func (t *failingStartTask) Stop(ctx context.Context) error {
	return nil
}

type blockingTask struct {
	name string
	stop chan struct{}
}

func newBlockingTask(name string) *blockingTask {
	return &blockingTask{
		name: name,
		stop: make(chan struct{}),
	}
}

func (t *blockingTask) Name() string {
	return t.name
}

func (t *blockingTask) StartWithContext(opts StartOptions) error {
	if opts.Ready != nil {
		opts.Ready.Done()
	}
	<-t.stop
	return nil
}

func (t *blockingTask) Stop(ctx context.Context) error {
	select {
	case <-t.stop:
		return nil
	default:
		close(t.stop)
		return nil
	}
}

type delayedStartTask struct {
	name string
	stop chan struct{}
}

func newDelayedStartTask(name string) *delayedStartTask {
	return &delayedStartTask{
		name: name,
		stop: make(chan struct{}),
	}
}

func (t *delayedStartTask) Name() string {
	return t.name
}

func (t *delayedStartTask) StartWithContext(opts StartOptions) error {
	// Simulate a service that is still booting and has not become ready yet.
	<-t.stop
	return nil
}

func (t *delayedStartTask) Stop(ctx context.Context) error {
	select {
	case <-t.stop:
		return nil
	default:
		close(t.stop)
		return nil
	}
}

type testLogger struct{}

func (l testLogger) Infof(template string, args ...interface{}) {}

func (l testLogger) Errorf(template string, args ...interface{}) {}
