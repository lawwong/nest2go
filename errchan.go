package nest2go

import (
	"errors"
	"fmt"
)

type ErrChan chan error

func NewErrChan() ErrChan {
	return make(chan error)
}

func (e ErrChan) Send(err error) {
	if e != nil {
		e <- err
	}
}

func (e ErrChan) Sends(msg string) {
	if e != nil {
		e <- errors.New(msg)
	}
}

func (e ErrChan) Sendf(format string, args ...interface{}) {
	if e != nil {
		e <- fmt.Errorf(format, args...)
	}
}
