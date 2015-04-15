package nest2go

import (
	"bytes"
	"github.com/lawwong/nest2go/uuid"
	"sync"
)

type agentOpInfo struct {
	code   int32
	serial uint32
	data   []byte
}

type AgentBase struct {
	session       *uuid.Uuid
	app           *AppBase
	serial        uint32 // only access in *AppBase.verifyAgent
	lastApplyTime int64

	peer      *PeerBase
	peerOpMux *opReqMux

	cacheMut     sync.RWMutex
	cacheReq     agentOpInfo
	cacheResData []byte

	setpeer chan *PeerBase
	writeop chan agentOpInfo

	*closable
}

func newAgentBase(session *uuid.Uuid, app *AppBase) *AgentBase {
	a := &AgentBase{
		session:   session,
		app:       app,
		peerOpMux: newOpReqMux(),

		setpeer: make(chan *PeerBase),
		writeop: make(chan agentOpInfo),

		closable: newClosable(),
	}
	go a.run()
	return a
}

func (a *AgentBase) run() {
	var peer *PeerBase
	for {
		select {
		case newPeer := <-a.setpeer:
			if peer != nil {
				newPeer.encodeKey = peer.encodeKey
				newPeer.decodeKey = peer.decodeKey
				newPeer.opSerial = peer.opSerial
				peer.Close()
			}
			if newPeer != nil {
				newPeer.oMux = a.peerOpMux
				// peer remover
				go func(cloze <-chan struct{}) {
					<-cloze
					a.setPeer(nil)
				}(peer.CloseChan())
			}
			peer = newPeer
		case info := <-a.writeop:
			// cache response
			a.cacheMut.Lock()
			if a.cacheReq.code == info.code &&
				a.cacheReq.serial == info.serial {
				a.cacheResData = info.data
			}
			a.cacheMut.Unlock()

			if peer != nil {
				peer.WriteOpRes(info.code, info.serial, info.data)
			}
		case <-a.CloseChan():
			break
		}
	}
}

// return replaced peer(closed)
func (a *AgentBase) setPeer(p *PeerBase) {
	select {
	case a.setpeer <- p:
	case <-a.CloseChan():
	}
}

func (a *AgentBase) WriteOpRes(code int32, serial uint32, data []byte) error {
	select {
	case a.writeop <- agentOpInfo{
		code:   code,
		serial: serial,
		data:   data,
	}:
		return nil
	case <-a.CloseChan():
		return ErrServiceClosed
	}
}

func (a *AgentBase) HandleOpReq(code int32, handler OpReqHandler) {
	a.peerOpMux.HandleOpReqFunc(
		code,
		func(res *OpResWriter, data []byte) {
			a.cacheMut.Lock()
			cacheReq := a.cacheReq
			cacheResData := a.cacheResData

			if res.serial > cacheReq.serial {
				// write cache
				a.cacheReq = agentOpInfo{
					code:   res.code,
					serial: res.serial,
					data:   data,
				}
				a.cacheResData = nil
				a.cacheMut.Unlock()
				// hijack
				res.rw = a

				handler.ServeOpReq(res, data)
			} else {
				a.cacheMut.Unlock()
				if res.serial == cacheReq.serial {
					if res.code != cacheReq.code {
						a.app.log.Warnf("wrong operation code")
					} else if cacheResData == nil || !bytes.Equal(data, cacheReq.data) {
						a.app.log.Warnf("wrong operation data")
					} else {
						a.WriteOpRes(res.code, res.serial, cacheResData)
					}
				} else {
					a.app.log.Warnf("wrong operation serial")
				}
			}
		},
	)
}

func (a *AgentBase) HandleOpReqFunc(code int32, handler OpReqHandlerFunc) {
	a.peerOpMux.HandleOpReqFunc(code, handler)
}

func (a *AgentBase) HandleClose(handler CloseHandler) {
	a.closable.HandleCloseFunc(func() {
		a.setPeer(nil)
		handler.HandleClose()
	})
}
