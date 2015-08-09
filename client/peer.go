package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha1"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/datapkg"
	. "github.com/lawwong/nest2go/protomsg"
)

var (
	hsModePeer     = HandshakeReq_PEER.Enum()
	hsModeNewAgent = HandshakeReq_NEW_AGENT.Enum()
	hsModeAgent    = HandshakeReq_AGENT.Enum()
)

type Peer struct {
	ws *websocket.Conn

	decodeKey    []byte
	encodeKey    []byte
	decodeStream cipher.Stream
	encodeStream cipher.Stream

	eventChan chan EventData
	opChan    chan OpResponse
	errChan   chan error

	writeMsg chan []byte
	writeErr chan error

	Closable
}

func newPeer(addr, appName string, useTls bool, hsReq *HandshakeReq, publicKey *rsa.PublicKey) (*Peer, *HandshakeRes, error) {
	var conn *websocket.Conn
	var err error

	if useTls {
		conn, _, err = websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/%s", addr, appName), nil)
	} else {
		conn, _, err = websocket.DefaultDialer.Dial(fmt.Sprintf("ws://%s/%s", addr, appName), nil)
	}
	if err != nil {
		return nil, nil, err
	}

	var challengeCode int32 = 12345
	hsReq.Challenge = &challengeCode

	hsReqData, err := proto.Marshal(hsReq)
	if err != nil {
		return nil, nil, err
	}

	if publicKey != nil {
		// encrypt with public key
		hsReqData, err = rsa.EncryptOAEP(sha1.New(), nil, publicKey, hsReqData, nil)
		if err != nil {
			return nil, nil, err
		}
	}

	err = conn.WriteMessage(websocket.BinaryMessage, hsReqData)
	if err != nil {
		return nil, nil, err
	}

	_, hsResData, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, err
	}

	p := &Peer{
		ws: conn,

		eventChan: make(chan EventData),
		opChan:    make(chan OpResponse),
		errChan:   make(chan error),

		writeMsg: make(chan []byte),
		writeErr: make(chan error),

		Closable: MakeClosable(),
	}

	if publicKey != nil {
		// make aes key
		p.decodeKey = hsReq.GetAesKey()
		decodeBlock, err := aes.NewCipher(p.decodeKey)
		if err != nil {
			return nil, nil, err
		}
		p.decodeStream = cipher.NewCTR(decodeBlock, hsReq.GetAesIv())
		// decode with aes key
		p.decodeStream.XORKeyStream(hsResData, hsResData)
	}

	hsRes := new(HandshakeRes)
	err = proto.Unmarshal(hsResData, hsRes)
	if err != nil {
		return nil, nil, err
	}

	if hsRes.GetChallenge() != challengeCode {
		return nil, nil, fmt.Errorf("Wrong challenge code")
	}

	if publicKey != nil {
		p.encodeKey = hsRes.GetAesKey()
		encodeBlock, err := aes.NewCipher(p.encodeKey)
		if err != nil {
			return nil, nil, err
		}
		p.encodeStream = cipher.NewCTR(encodeBlock, hsRes.GetAesIv())
	}

	go p.runReader()
	go p.runWriter()

	return p, hsRes, nil
}

func (p *Peer) runReader() {
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
			//p.app.log.Warnf("%q peer %q : %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
			continue
		}

		switch msg.GetType() {
		case DataMsg_EVENT:
			p.eventChan <- MakeEventData(
				msg.GetCode(),
				msg.GetData(),
			)
		case DataMsg_OP:
			p.opChan <- MakeOpResponse(
				msg.GetCode(),
				msg.GetResCode(),
				msg.GetSerial(),
				msg.GetData(),
			)
		}
	}
	close(p.eventChan)
	close(p.opChan)
	p.Close()
}

func (p *Peer) runWriter() {
	for {
		select {
		case data := <-p.writeMsg:
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

func (p *Peer) SendEvent(code int32, data []byte) error {
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

func (p *Peer) SendOpRequest(code int32, data []byte, serial int32) error {
	msg := DataMsg{
		Type: DataMsgTypeOp,
		Code: &code,
		Data: data,
	}

	if serial != 0 {
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
