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

	opSerial uint32

	eMux *eventMux
	oMux *opReqMux

	writeMsg chan []byte
	writeErr chan error

	*closable
}

func newPeerBase(ws *websocket.Conn, app *AppBase, privateKey *rsa.PrivateKey) *PeerBase {
	p := &PeerBase{
		ws:  ws,
		app: app,

		writeMsg: make(chan []byte),
		writeErr: make(chan error),

		closable: newClosable(),
	}
	p.HandleCloseFunc(func() {})
	go p.runHandshake(privateKey)
	return p
}

func (p *PeerBase) runHandshake(privateKey *rsa.PrivateKey) {
	var data []byte
	var err error
	var notifyAppFunc func()
	handshakeOk := make(chan Signal)
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
			p.eMux = newEventMux()
			p.oMux = newOpReqMux()

			// notify new peer to app
			notifyAppFunc = func() {
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
			agent := p.app.newAgent()
			agent.setPeer(p)
			// notify new agent to app
			notifyAppFunc = func() {
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
			notifyAppFunc()
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
	emptyData := make([]byte, 0)
	for {
		if err != nil {
			p.app.log.Warnf("%q peer %q : %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
		}

		_, data, err = p.ws.ReadMessage()
		if err != nil {
			break
		}

		if p.decodeStream != nil {
			p.decodeStream.XORKeyStream(data, data)
		}

		err = proto.Unmarshal(data, msg)
		if err != nil {
			continue
		}

		switch msg.GetType() {
		case DataMsg_EVENT:
			if p.eMux == nil {
				err = ErrAgentEventNotSupport
			} else {
				err = p.eMux.serveEvent(msg.GetCode(), msg.GetData())
			}
		case DataMsg_OP:
			err = p.oMux.serveOpReq(p, msg.GetCode(), msg.GetOpSerial(), msg.GetData())
			if err != nil {
				*msg = DataMsg{
					Type:     DataMsg_WRONG_OP_CODE.Enum(),
					Code:     proto.Int32(msg.GetCode()),
					OpSerial: proto.Uint32(msg.GetOpSerial()),
					Data:     emptyData,
				}
				data, _ = proto.Marshal(msg)
				select {
				case p.writeMsg <- data:
					<-p.writeErr
				case <-p.CloseChan():
				}
			}
		}
	}
	p.app.log.Infof("%q peer %q : %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
	p.Close()
}

func (p *PeerBase) runWriter(hsResData []byte) {
	var handshakeOk bool
	for {
		select {
		case data := <-p.writeMsg:
			if !handshakeOk {
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
					handshakeOk = true
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

func (p *PeerBase) WriteEvent(code int32, data []byte) error {
	msg := DataMsg{
		Type: DataMsg_EVENT.Enum(),
		Code: proto.Int32(code),
		Data: data,
	}
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	select {
	case p.writeMsg <- data:
		return <-p.writeErr
	case <-p.CloseChan():
		return ErrServiceClosed
	}
}

func (p *PeerBase) WriteOpRes(code int32, serial uint32, data []byte) error {
	msg := DataMsg{
		Type:     DataMsg_OP.Enum(),
		Code:     proto.Int32(code),
		OpSerial: proto.Uint32(serial),
		Data:     data,
	}
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	select {
	case p.writeMsg <- data:
		return <-p.writeErr
	case <-p.CloseChan():
		return ErrServiceClosed
	}
}

func (p *PeerBase) HandleEvent(code int32, handler EventHandler) {
	p.eMux.HandleEvent(code, handler)
}

func (p *PeerBase) HandleEventFunc(code int32, handler EventHandlerFunc) {
	p.eMux.HandleEventFunc(code, handler)
}

func (p *PeerBase) HandleOpReq(code int32, handler OpReqHandler) {
	p.oMux.HandleOpReq(code, handler)
}

func (p *PeerBase) HandleOpReqFunc(code int32, handler OpReqHandlerFunc) {
	p.oMux.HandleOpReqFunc(code, handler)
}

func (p *PeerBase) HandleClose(handler CloseHandler) {
	p.closable.HandleCloseFunc(func() {
		if err := p.ws.Close(); err != nil {
			p.app.log.Infof("%q peer %q closed. %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
		}
		handler.HandleClose()
	})
}
