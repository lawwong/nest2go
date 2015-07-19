package closable

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

type Closable struct {
	handlerMut sync.RWMutex
	handler    CloseHandler
	closedMut  sync.RWMutex
	closed     bool
	cloze      chan struct{}
}

func MakeClosable() Closable {
	return Closable{
		cloze: make(chan struct{}),
	}
}

//func newClosable() *closable {
//	c := new(closable)
//	*c = makeClosable()
//	return c
//}

func (c *Closable) HandleClose(handler CloseHandler) {
	c.closedMut.Lock()
	defer c.closedMut.Unlock()
	c.handler = handler
}

func (c *Closable) HandleCloseFunc(handleFunc CloseHandlerFunc) {
	c.HandleClose(CloseHandler(handleFunc))
}

//func (c *closable) Close() error {
//	c.closedMut.Lock()
//	defer c.closedMut.Unlock()
//	if c.closed {
//		return ErrAlreadyClosed
//	}
//	c.closed = true

//	go func() {
//		c.handlerMut.RLock()
//		defer c.handlerMut.RUnlock()
//		if c.handler != nil {
//			c.handler.HandleClose()
//		}
//		close(c.cloze)
//	}()
//	return nil
//}

func (c *Closable) LockClosedMut() {
	c.closedMut.Lock()
}

func (c *Closable) UnlockClosedMut() {
	c.closedMut.Unlock()
}

func (c *Closable) RLockClosedMut() {
	c.closedMut.RLock()
}

func (c *Closable) RUnlockClosedMut() {
	c.closedMut.RUnlock()
}

func (c *Closable) Closed() bool {
	return c.closed
}

func (c *Closable) Close() {
	c.closedMut.Lock()
	defer c.closedMut.Unlock()
	if c.closed {
		return
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
}

func (c *Closable) CloseChan() <-chan struct{} {
	return c.cloze
}
