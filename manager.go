package srvmgr

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

type Manager struct {
	MaxWaitingStop  time.Duration
	underlyingTasks []*underlyingTask
	logger          Logger
	onAllUp         func()
	stopOnce        sync.Once
	mainSignal      chan os.Signal
}

func NewManager(logger Logger, maxWaitStop time.Duration, onAllUp func()) *Manager {
	return &Manager{
		underlyingTasks: []*underlyingTask{},
		logger:          logger,
		MaxWaitingStop:  maxWaitStop,
		onAllUp:         onAllUp,
		stopOnce:        sync.Once{},
		mainSignal:      make(chan os.Signal, 1),
	}
}

func (m *Manager) AddTask(tasks ...Task) {
	for _, t := range tasks {
		underTask := &underlyingTask{
			task:     t,
			done:     make(chan struct{}),
			stopOnce: sync.Once{},
		}
		m.underlyingTasks = append(m.underlyingTasks, underTask)
	}
}

func (m *Manager) Run() error {
	stopAll := func() {
		ctx, _ := context.WithTimeout(context.Background(), m.MaxWaitingStop)
		// no need to return here since all errors will be logged
		_ = m.Shutdown(ctx)
	}

	startup := NewErroWaitGroup(len(m.underlyingTasks))
	var allTasks errgroup.Group
	for _, under := range m.underlyingTasks {
		underTask := under
		allTasks.Go(func() error {
			defer stopAll()
			ready := &taskReady{group: startup}

			m.logger.Infof("[%s]: starting task", underTask.task.Name())
			err := underTask.task.StartWithContext(StartOptions{Ready: ready})
			if err != nil {
				if !ready.IsDone() {
					startup.Fail(err)
				}
				m.logger.Infof("[%s] interrupted: %s", underTask.task.Name(), err)
				return err
			}
			if !ready.IsDone() {
				// Task stopped without signaling readiness.
				startup.Fail(errors.New("task exited before signaling ready"))
			}
			return nil
		})
	}

	allTasks.Go(func() error {
		defer stopAll()
		signal.Notify(m.mainSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		select {
		case _, ok := <-m.mainSignal:
			if ok {
				m.logger.Infof("external request to stop")
			}
		}
		return nil
	})

	if err := startup.Wait(); err != nil {
		stopAll()
		return err
	}
	if m.onAllUp != nil {
		m.onAllUp()
	}

	return allTasks.Wait()
}

func (m *Manager) Shutdown(ctx context.Context) error {
	var err error

	m.stopOnce.Do(func() {
		close(m.mainSignal)
		var allTasks errgroup.Group
		for _, under := range m.underlyingTasks {
			underTask := under
			allTasks.Go(func() error {
				m.logger.Infof("[%s]: stopping task", underTask.task.Name())
				if err := underTask.task.Stop(ctx); err != nil {
					m.logger.Errorf("[%s] error stopping task: %s", underTask.task.Name(), err)
					return err
				}
				m.logger.Infof("[%s]: task stopped", underTask.task.Name())
				return nil
			})
		}
		err = allTasks.Wait()
	})
	return err
}

type underlyingTask struct {
	task     Task
	done     chan struct{}
	stopOnce sync.Once
}

type taskReady struct {
	group *ErroWaitGroup
	once  sync.Once
	done  bool
	mu    sync.Mutex
}

func (r *taskReady) Done() {
	r.once.Do(func() {
		r.mu.Lock()
		r.done = true
		r.mu.Unlock()
		r.group.Done()
	})
}

func (r *taskReady) IsDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

func (u *underlyingTask) Run() error {
	defer close(u.done)
	return u.task.StartWithContext(StartOptions{})
}

func (u *underlyingTask) Shutdown(ctx context.Context) error {
	var err error
	u.stopOnce.Do(func() {
		go func() {
			err = u.task.Stop(ctx)
			if err != nil {
				return
			}
		}()

		select {
		case <-u.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}
