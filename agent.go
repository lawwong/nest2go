package nest2go

import (
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/datapkg"
	"github.com/lawwong/nest2go/uuid"
)

type agentResInfo struct {
	req     OpRequest
	resCode int32
	data    []byte
}

type AgentBase struct {
	session       *uuid.Uuid
	app           *AppBase
	serial        int32 // only access in *AppBase.verifyAgent
	lastApplyTime int64

	peer *PeerBase

	setpeer chan *PeerBase
	opChan  chan OpRequest

	writeOp chan agentResInfo

	Closable
}

func (a *AgentBase) OpChan() <-chan OpRequest {
	if a != nil {
		return a.opChan
	}
	return nil
}

func newAgentBase(session *uuid.Uuid, app *AppBase, peer *PeerBase) *AgentBase {
	a := &AgentBase{
		session: session,
		app:     app,

		opChan: make(chan OpRequest),

		setpeer: make(chan *PeerBase),
		writeOp: make(chan agentResInfo),

		Closable: MakeClosable(),
	}
	a.HandleClose(nil)
	go a.run(peer)
	return a
}

func (a *AgentBase) run(peer *PeerBase) {
	// peer initial
	encodeKey := peer.encodeKey
	decodeKey := peer.decodeKey
	eventChan := peer.EventChan()
	opChan := peer.OpChan()

	var resInfoCache agentResInfo
	//var resMsgCache []byte

	for {
		select {
		case newPeer := <-a.setpeer:
			if newPeer != nil {
				// peer replaced
				newPeer.encodeKey = encodeKey
				newPeer.decodeKey = decodeKey
				eventChan = newPeer.EventChan()
				opChan = newPeer.OpChan()
			} else {
				// peer removed
				eventChan = nil
				opChan = nil
			}

			if peer != nil {
				peer.Close()
			}

			peer = newPeer
		case _, ok := <-eventChan:
			if !ok {
				eventChan = nil
			}
			// not support
		case opReq, ok := <-opChan:
			if !ok {
				opChan = nil
			} else {
				// read request from peer
				opReq.SetResponseSender(a)
				if opReq.IsEqual(resInfoCache.req) {
					if peer != nil { // peer must not be nil?
						peer.SendOpResponse(resInfoCache.req, resInfoCache.resCode, resInfoCache.data)
					}
				} else {
					select {
					case a.opChan <- opReq:
					case <-a.CloseChan():
						break
					}
				}
			}
		case resInfo := <-a.writeOp:
			resInfoCache = resInfo
			// TODO : serialized cache??

			if peer != nil {
				peer.SendOpResponse(resInfo.req, resInfo.resCode, resInfo.data)
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

func (a *AgentBase) SendOpResponse(req OpRequest, resCode int32, data []byte) error {
	select {
	case a.writeOp <- agentResInfo{
		req,
		resCode,
		data,
	}:
		return nil
	case <-a.CloseChan():
		return ErrServiceClosed
	}
}

//func (a *AgentBase) HandleClose(handler CloseHandler) {
//	a.closable.HandleCloseFunc(func() {
//		a.setPeer(nil)
//		if handler != nil {
//			handler.HandleClose()
//		}
//	})
//}

//func (a *AgentBase) HandleCloseFunc(handleFunc CloseHandlerFunc) {
//	a.HandleClose(CloseHandler(handleFunc))
//}
