package srvmgr

import (
	"context"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Manager struct {
	MaxWaitingStop  time.Duration
	underlyingTasks []*underlyingTask
	logger          Logger
	stopOnce        sync.Once
	mainSignal      chan os.Signal
}

func NewManager(logger Logger, maxWaitStop time.Duration) *Manager {
	return &Manager{
		underlyingTasks: []*underlyingTask{},
		logger:          logger,
		MaxWaitingStop:  maxWaitStop,
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

	var allTasks errgroup.Group
	for _, under := range m.underlyingTasks {
		underTask := under
		allTasks.Go(func() error {
			defer stopAll()

			m.logger.Infof("[%s]: starting task", underTask.task.Name())
			if err := underTask.task.Start(); err != nil {
				m.logger.Infof("[%s] interrupted: %s", underTask.task.Name(), err)
				return err
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

func (u *underlyingTask) Run() error {
	defer close(u.done)
	return u.task.Start()
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
