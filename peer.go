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
)

type PeerListener interface {
}

const BUFFER_SIZE = 256

type PeerBase struct {
	ws  *websocket.Conn
	app *AppBase

	listener PeerListener

	rsaKey    *rsa.PrivateKey
	decodeKey []byte
	encodeKey []byte

	read  chan []byte
	write chan []byte
	stop  chan struct{}
}

func newPeerBase(ws *websocket.Conn, app *AppBase, pk *rsa.PrivateKey) *PeerBase {
	p := &PeerBase{
		ws:     ws,
		app:    app,
		rsaKey: pk,

		read:  make(chan []byte, BUFFER_SIZE),
		write: make(chan []byte, BUFFER_SIZE),
		stop:  make(chan struct{}),
	}
	go p.runReader()
	return p
}

func (p *PeerBase) runReader() {
	var data []byte
	var err error
	var handshake bool
	var decodeStream cipher.Stream
	var encodeStream cipher.Stream
	defer p.app.server.printErr(&err, "PeerBase.runReader", p.ws.LocalAddr(), p.ws.RemoteAddr())

	for _, data, err = p.ws.ReadMessage(); err != nil; {
		if !handshake {
			// do handshake
			if p.useSecure() {
				// decode by rsa private key
				data, err = rsa.DecryptOAEP(sha1.New(), nil, p.rsaKey, data, nil)
				if err != nil {
					break
				}
			}
			// parse message
			hsReq := new(HandshakeReq)
			err = proto.Unmarshal(data, hsReq)
			if err != nil {
				break
			}
			// build HandshakeRes
			hsRes := new(HandshakeRes)
			// client should test this field if it's equal
			hsRes.Challenge = proto.Int32(hsReq.GetChallenge())
			// switch mode
			switch hsReq.GetMode() {
			case HandshakeReq_PEER:
				if l := p.app.listener.IncomingPeer(p); l == nil {
					err = fmt.Errorf("Peer reject by app!")
					break
				} else {
					p.listener = l
				}

				if p.useSecure() {
					p.decodeKey = hsReq.GetAesKey()
					p.encodeKey = make([]byte, len(p.decodeKey))
					_, err = rand.Read(p.encodeKey)
					if err != nil {
						return
					}

					hsRes.AesKey = p.encodeKey
				}
			case HandshakeReq_NEW_AGENT:
				agent := p.app.newAgent(hsReq.GetUtcTime())
				// TODO
				if agent == nil {
					err = fmt.Errorf("Agent reject by app!")
					break
				}

				agent.setPeer(p)
				//p.listener =

				hsRes.Session = agent.session.ByteSlice()

				if p.useSecure() {
					p.decodeKey = hsReq.GetAesKey()
					p.encodeKey = make([]byte, len(p.decodeKey))
					_, err = rand.Read(p.encodeKey)
					if err != nil {
						return
					}

					hsRes.AesKey = p.encodeKey
				}
			case HandshakeReq_AGENT:
				session, err := uuid.FromByteSlice(hsReq.GetSession())
				if err != nil {
					break
				}

				agent, err := p.app.verifyAgent(session, hsReq.GetUtcTime())
				if err != nil {
					break
				}

				oldPeer := agent.setPeer(p)

				if p.useSecure() {
					p.decodeKey = oldPeer.decodeKey
					p.encodeKey = oldPeer.encodeKey
				}
			}

			// build cipher stream
			if p.useSecure() {
				decodeBlock, err := aes.NewCipher(p.decodeKey)
				if err != nil {
					return
				}

				encodeBlock, err := aes.NewCipher(p.encodeKey)
				if err != nil {
					return
				}

				decodeIv := hsReq.GetAesIv()
				if len(decodeIv) != aes.BlockSize {
					err = fmt.Errorf("IV length must equal block size!")
					break
				}

				encodeIv := make([]byte, aes.BlockSize)
				_, err = rand.Read(encodeIv)
				if err != nil {
					break
				}

				decodeStream = cipher.NewCTR(decodeBlock, decodeIv)
				encodeStream = cipher.NewCTR(encodeBlock, encodeIv)

				hsRes.AesIv = encodeIv
			}

			data, err = proto.Marshal(hsRes)
			if err != nil {
				break
			}

			go p.runWriter(encodeStream)

			p.write <- data

			handshake = true
		} else {
			if decodeStream != nil {
				decodeStream.XORKeyStream(data, data)
			}

		}
	}
	p.ws.Close()
	close(p.write)
	//close(p.stop)
}

func (p *PeerBase) runWriter(encodeStream cipher.Stream) {
	for data := range p.write {
		if encodeStream != nil {
			encodeStream.XORKeyStream(data, data)
		}

		if err := p.ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
			p.app.server.log.Warnf("PeerBase.runWriter(%s,%s,) %s", p.ws.LocalAddr(), p.ws.RemoteAddr(), err)
		}
	}
}

func (p *PeerBase) run() {
	for {
		select {
		case <-p.stop:
			break
		}
	}
	close(p.write)
}

func (p *PeerBase) useSecure() bool {
	return p.rsaKey != nil
}

func (p *PeerBase) Close() (err error) {
	defer p.app.server.printErr(&err, "PeerBase.Close", p.ws.LocalAddr(), p.ws.RemoteAddr())
	err = p.ws.Close()
	return
}
