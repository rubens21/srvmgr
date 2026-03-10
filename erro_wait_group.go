package srvmgr

import "sync"

type ErroWaitGroup struct {
	wg     sync.WaitGroup
	done   chan struct{}
	errCh  chan error
	errOne sync.Once
}

func NewErroWaitGroup(total int) *ErroWaitGroup {
	group := &ErroWaitGroup{
		done:  make(chan struct{}),
		errCh: make(chan error, 1),
	}
	group.wg.Add(total)
	go func() {
		group.wg.Wait()
		close(group.done)
	}()
	return group
}

func (g *ErroWaitGroup) Done() {
	g.wg.Done()
}

func (g *ErroWaitGroup) Fail(err error) {
	if err == nil {
		return
	}
	g.errOne.Do(func() {
		g.errCh <- err
	})
}

func (g *ErroWaitGroup) Wait() error {
	select {
	case <-g.done:
		return nil
	case err := <-g.errCh:
		return err
	}
}
