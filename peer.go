package nest2go

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/datapkg"
	. "github.com/lawwong/nest2go/protomsg"
	"github.com/lawwong/nest2go/uuid"
	"time"
)

type PeerBase struct {
	ws  *websocket.Conn
	app *AppBase

	decodeKey    []byte
	encodeKey    []byte
	decodeStream cipher.Stream
	encodeStream cipher.Stream

	eventChan chan EventData
	opChan    chan OpRequest

	writeMsg chan []byte
	writeErr chan error

	Closable
}

func (p *PeerBase) EventChan() <-chan EventData {
	if p != nil {
		return p.eventChan
	}
	return nil
}

func (p *PeerBase) OpChan() <-chan OpRequest {
	if p != nil {
		return p.opChan
	}
	return nil
}

func newPeerBase(ws *websocket.Conn, app *AppBase, privateKey *rsa.PrivateKey) *PeerBase {
	p := &PeerBase{
		ws:  ws,
		app: app,

		eventChan: make(chan EventData),
		opChan:    make(chan OpRequest),

		writeMsg: make(chan []byte),
		writeErr: make(chan error),

		Closable: MakeClosable(),
	}
	p.HandleClose(nil)
	go p.runHandshake(privateKey)
	return p
}

func (p *PeerBase) runHandshake(privateKey *rsa.PrivateKey) {
	var data []byte
	var err error
	var notifyAppFunc func(*PeerBase)
	handshakeOk := make(chan struct{})
	defer printErr(p.app.log, &err, "PeerBase.runHandshake", p.ws.LocalAddr(), p.ws.RemoteAddr())
	defer func() {
		if err != nil {
			p.Close()
		} else {
			close(handshakeOk)
		}
	}()

	go func() {
		select {
		case <-handshakeOk:
		case <-time.After(2 * time.Second):
			p.Close()
		}
	}()

	if _, data, err = p.ws.ReadMessage(); err == nil {
		// do handshake
		if privateKey != nil {
			// decode with rsa private key
			data, err = rsa.DecryptOAEP(sha1.New(), nil, privateKey, data, nil)
			if err != nil {
				return
			}
		}
		// parse message
		hsReq := new(HandshakeReq)
		err = proto.Unmarshal(data, hsReq)
		if err != nil {
			return
		}
		// build HandshakeRes
		hsRes := new(HandshakeRes)
		// client should test if it's equal
		hsRes.Challenge = proto.Int32(hsReq.GetChallenge())
		// switch mode
		switch hsReq.GetMode() {
		case HandshakeReq_PEER:
			//p.eMux = newEventMux()
			//p.oMux = newOpReqMux()

			// notify new peer to app
			notifyAppFunc = func(p *PeerBase) {
				select {
				case p.app.onpeer <- p:
				case <-p.app.CloseChan():
					p.Close()
				case <-p.CloseChan():
				}
			}

			if privateKey != nil {
				p.encodeKey = hsReq.GetAesKey()
				p.decodeKey = make([]byte, len(p.encodeKey))
				_, err = rand.Read(p.decodeKey)
				if err != nil {
					return
				}

				hsRes.AesKey = p.decodeKey
			}
		case HandshakeReq_NEW_AGENT:
			agent := p.app.newAgent(p)
			//agent.setPeer(p)
			// notify new agent to app
			notifyAppFunc = func(p *PeerBase) {
				select {
				case p.app.onagent <- agent:
				case <-p.app.CloseChan():
					p.Close()
				case <-p.CloseChan():
				}
			}

			hsRes.Session = agent.session.ByteSlice()

			if privateKey != nil {
				p.encodeKey = hsReq.GetAesKey()
				p.decodeKey = make([]byte, len(p.encodeKey))
				_, err = rand.Read(p.decodeKey)
				if err != nil {
					return
				}

				hsRes.AesKey = p.decodeKey
			}
		case HandshakeReq_AGENT:
			var session *uuid.Uuid
			*session, err = uuid.FromByteSlice(hsReq.GetSession())
			if err != nil {
				return
			}

			agent, err := p.app.verifyAgent(session, hsReq.GetSerialNum())
			if err != nil {
				return
			}

			agent.setPeer(p)
		}

		// make cipher stream
		if privateKey != nil {
			encodeBlock, err := aes.NewCipher(p.encodeKey)
			if err != nil {
				return
			}

			decodeBlock, err := aes.NewCipher(p.decodeKey)
			if err != nil {
				return
			}

			encodeIv := hsReq.GetAesIv()
			if len(encodeIv) != aes.BlockSize {
				err = fmt.Errorf("IV length must equal block size!")
				return
			}

			decodeIv := make([]byte, aes.BlockSize)
			_, err = rand.Read(decodeIv)
			if err != nil {
				return
			}

			// FIXME : thread safe?
			p.encodeStream = cipher.NewCTR(encodeBlock, encodeIv)
			p.decodeStream = cipher.NewCTR(decodeBlock, decodeIv)

			hsRes.AesIv = decodeIv
		}

		data, err = proto.Marshal(hsRes)
		if err != nil {
			return
		}

		if p.encodeStream != nil {
			p.encodeStream.XORKeyStream(data, data)
		}

		go p.runWriter(data)

		if notifyAppFunc != nil {
			// app can handle event/op, send event/op, close peer/agent
			notifyAppFunc(p)
		}

		// try send handshake response
		p.writeMsg <- nil
		<-p.writeErr

		go p.runReader()
	}
}

func (p *PeerBase) runReader() {
	var data []byte
	var err error
	msg := new(DataMsg)
	for {
		_, data, err = p.ws.ReadMessage()
		if err != nil {
			break
		}

		if p.decodeStream != nil {
			p.decodeStream.XORKeyStream(data, data)
		}

		err = proto.Unmarshal(data, msg)
		if err != nil {
			p.app.log.Warnf("%q peer %q : %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
			continue
		}

		switch msg.GetType() {
		case DataMsg_EVENT:
			p.eventChan <- MakeEventData(
				msg.GetCode(),
				msg.GetData(),
			)
		case DataMsg_OP:
			p.opChan <- MakeOpRequest(
				msg.GetCode(),
				msg.GetSerial(),
				msg.GetData(),
				p,
			)
		}
	}
	p.app.log.Infof("%q peer %q : %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
	close(p.eventChan)
	close(p.opChan)
	p.Close()
}

func (p *PeerBase) runWriter(hsResData []byte) {
	//msgSlice =:= make([]*DataMsg, 0, 128)
	for {
		select {
		case data := <-p.writeMsg:
			if hsResData != nil {
				// make sure handshake response sent before first message,
				// and let application have a chance to handle event/op before
				// client received handshake response,
				// hsResData should be encrypted if needed
				err := p.ws.WriteMessage(websocket.BinaryMessage, hsResData)
				if err != nil {
					p.writeErr <- err
					continue
				} else {
					hsResData = nil
				}
			}

			if len(data) == 0 {
				p.writeErr <- nil
			} else {
				if p.encodeStream != nil {
					p.encodeStream.XORKeyStream(data, data)
				}
				p.writeErr <- p.ws.WriteMessage(websocket.BinaryMessage, data)
			}
		case <-p.CloseChan():
			break
		}
	}
	close(p.writeErr)
}

func (p *PeerBase) SendEvent(code int32, data []byte) error {
	msg := DataMsg{
		Type: DataMsgTypeEvent,
		Code: &code,
		Data: data,
	}
	d, _ := proto.Marshal(&msg) // must succeed

	select {
	case p.writeMsg <- d:
		return <-p.writeErr
	case <-p.CloseChan():
		return ErrServiceClosed
	}
}

func (p *PeerBase) SendOpResponse(req OpRequest, resCode int32, data []byte) error {
	var code int32 = req.Code()
	var serial int32 = req.Serial()

	msg := DataMsg{
		Type:    DataMsgTypeOp,
		Code:    &code,
		ResCode: &resCode,
		Data:    data,
	}

	if serial = req.Serial(); serial != 0 {
		msg.Serial = &serial
	}

	d, _ := proto.Marshal(&msg) // must succeed

	select {
	case p.writeMsg <- d:
		return <-p.writeErr
	case <-p.CloseChan():
		return ErrServiceClosed
	}
}

func (p *PeerBase) HandleClose(handler CloseHandler) {
	p.Closable.HandleCloseFunc(func() {
		if err := p.ws.Close(); err != nil {
			p.app.log.Infof("%q peer %q closed. %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
		}
		if handler != nil {
			handler.HandleClose()
		}
	})
}

func (p *PeerBase) HandleCloseFunc(handleFunc CloseHandlerFunc) {
	p.HandleClose(CloseHandler(handleFunc))
}

func BroadcastEventByChan(peerChan <-chan *PeerBase, code int32, data []byte) {
	if peerChan == nil {
		return
	}
	go func() {
		msg := DataMsg{
			Type: DataMsgTypeEvent,
			Code: proto.Int32(code),
			Data: data,
		}
		d, _ := proto.Marshal(&msg) // must succeed
		for p := range peerChan {
			if p != nil {
				go func(p *PeerBase) {
					select {
					case p.writeMsg <- d:
						<-p.writeErr
					case <-p.CloseChan():
					}
				}(p)
			}
		}
	}()
}

func BroadcastEvent(peers []*PeerBase, code int32, data []byte) {
	if len(peers) == 0 {
		return
	}
	peerChan := make(chan *PeerBase)
	BroadcastEventByChan(peerChan, code, data)
	for _, p := range peers {
		peerChan <- p
	}
	close(peerChan)
}
