package nest2go

import (
	"fmt"
	"sync"
)

var ErrAlreadyClosed = fmt.Errorf("already closed")

type CloseHandler interface {
	HandleClose()
}

type CloseHandlerFunc func()

func (h CloseHandlerFunc) HandleClose() {
	h()
}

type closable struct {
	handlerMut sync.RWMutex
	handler    CloseHandler
	closedMut  sync.RWMutex
	closed     bool
	cloze      chan struct{}
}

func newClosable() *closable {
	return &closable{
		cloze: make(chan struct{}),
	}
}

func (c *closable) HandleClose(handler CloseHandler) {
	c.closedMut.Lock()
	defer c.closedMut.Unlock()
	c.handler = handler
}

func (c *closable) HandleCloseFunc(handleFunc CloseHandlerFunc) {
	c.HandleClose(CloseHandler(handleFunc))
}

func (c *closable) Close() error {
	c.closedMut.Lock()
	defer c.closedMut.Unlock()
	if c.closed {
		return ErrAlreadyClosed
	}
	c.closed = true

	go func() {
		c.handlerMut.RLock()
		defer c.handlerMut.RUnlock()
		if c.handler != nil {
			c.handler.HandleClose()
		}
		close(c.cloze)
	}()
	return nil
}

func (c *closable) CloseChan() <-chan struct{} {
	return c.cloze
}
