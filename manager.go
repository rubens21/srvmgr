package srvmgr

import (
	"context"
	"go.uber.org/zap"
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
	logger          *zap.SugaredLogger
	stopOnce        sync.Once
	mainSignal      chan os.Signal
}

func NewManager(logger *zap.SugaredLogger, maxWaitStop time.Duration) *Manager {
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
		localLogger := m.logger.With("task", underTask.task.Name())
		allTasks.Go(func() error {
			defer stopAll()

			localLogger.Info("starting task")
			if err := underTask.task.Start(); err != nil {
				localLogger.With("error", err).Error("interrupted")
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
				m.logger.Info("external request to stop")
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
			localLogger := m.logger.With("task", underTask.task.Name())
			allTasks.Go(func() error {
				localLogger.Info("stopping task")
				if err := underTask.task.Stop(ctx); err != nil {
					localLogger.With("error", err).Error("error stopping task")
					return err
				}
				localLogger.Info("task stopped")
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
