package nest2go

import (
	"fmt"
	"sync"
)

var (
	ErrAgentEventNotSupport = fmt.Errorf("agent event not suppe")
	ErrInvalidHandler       = fmt.Errorf("invalid handler")
	ErrHandlerNotRegistered = fmt.Errorf("handler not registered")
)

type EventHandler interface {
	ServeEvent(int32, []byte)
}

type EventHandlerFunc func(int32, []byte)

func (h EventHandlerFunc) ServeEvent(code int32, data []byte) {
	h(code, data)
}

type eventMux struct {
	mut sync.RWMutex
	m   map[int32]EventHandler
}

func newEventMux() *eventMux {
	return &eventMux{
		m: make(map[int32]EventHandler),
	}
}

func (m *eventMux) HandleEvent(code int32, handler EventHandler) {
	m.mut.Lock()
	defer m.mut.Lock()
	m.m[code] = handler
}

func (m *eventMux) HandleEventFunc(code int32, handler EventHandlerFunc) {
	m.HandleEvent(code, EventHandler(handler))
}

func (m *eventMux) serveEvent(code int32, data []byte) error {
	m.mut.RLock()
	defer m.mut.RUnlock()
	handler, found := m.m[code]
	if !found {
		return ErrHandlerNotRegistered
	}
	if handler == nil {
		return ErrInvalidHandler
	}
	handler.ServeEvent(code, data)
	return nil
}

type OpReqHandler interface {
	ServeOpReq(*OpResWriter, []byte)
}

type OpReqHandlerFunc func(*OpResWriter, []byte)

func (h OpReqHandlerFunc) ServeOpReq(w *OpResWriter, reqData []byte) {
	h(w, reqData)
}

type opReqMux struct {
	mut sync.RWMutex
	m   map[int32]OpReqHandler
}

func newOpReqMux() *opReqMux {
	return &opReqMux{
		m: make(map[int32]OpReqHandler),
	}
}

func (m *opReqMux) HandleOpReq(code int32, handler OpReqHandler) {
	m.mut.Lock()
	defer m.mut.Lock()
	m.m[code] = handler
}

func (m *opReqMux) HandleOpReqFunc(code int32, handler OpReqHandlerFunc) {
	m.HandleOpReq(code, OpReqHandler(handler))
}

func (m *opReqMux) serveOpReq(rw ResponseWriter, code int32, serial uint32, reqData []byte) error {
	m.mut.RLock()
	defer m.mut.RUnlock()
	handler, found := m.m[code]
	if !found {
		return ErrHandlerNotRegistered
	}
	if handler == nil {
		return ErrInvalidHandler
	}
	res := &OpResWriter{
		rw:     rw,
		code:   code,
		serial: serial,
	}
	handler.ServeOpReq(res, reqData)
	return nil
}

type ResponseWriter interface {
	WriteOpRes(int32, uint32, []byte) error
}

type OpResWriter struct {
	rw     ResponseWriter
	code   int32
	serial uint32
}

func (r *OpResWriter) Code() int32 {
	return r.code
}

func (r *OpResWriter) Serial() uint32 {
	return r.serial
}

func (r *OpResWriter) WriteData(data []byte) error {
	return r.rw.WriteOpRes(r.code, r.serial, data)
}
