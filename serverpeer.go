package nest2go

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	. "github.com/lawwong/nest2go/protomsg"
	"os"
	"time"
)

type ServerPeerBase struct {
	addr    string
	appName string
	ws      *websocket.Conn

	decodeKey    []byte
	encodeKey    []byte
	decodeStream cipher.Stream
	encodeStream cipher.Stream

	*closable
}

// aesKeyLen in byte
func ConnectToServer(addr, appName string, useTls bool, publicKey *rsa.PublicKey, aesKeyLen int) (*ServerPeerBase, error) {
	var ws *websocket.Conn
	var err error

	if useTls {
		ws, _, err = websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/%s", addr, appName), nil)
	} else {
		ws, _, err = websocket.DefaultDialer.Dial(fmt.Sprintf("ws://%s/%s", addr, appName), nil)
	}
	if err != nil {
		return nil, err
	}

	p := &ServerPeerBase{
		ws:       ws,
		closable: newClosable(),
	}

	err = p.peerHandshake(publicKey, aesKeyLen)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *ServerPeerBase) peerHandshake(publicKey *rsa.PublicKey, aesKeyLen int) (err error) {
	hsReq := &HandshakeReq{
		Mode:      HandshakeReq_PEER.Enum(),
		App:       proto.String(p.appName),
		Challenge: proto.Int32(int32(time.Now().UnixNano())),
	}

	if publicKey != nil {
		p.decodeKey = make([]byte, aesKeyLen)
		_, err = rand.Read(p.decodeKey)
		if err != nil {
			return
		}

		decodeIv := make([]byte, aes.BlockSize)
		_, err = rand.Read(decodeIv)
		if err != nil {
			return
		}

		var decodeBlock cipher.Block
		decodeBlock, err = aes.NewCipher(p.decodeKey)
		if err != nil {
			return
		}

		p.decodeStream = cipher.NewCTR(decodeBlock, decodeIv)

		hsReq.AesKey = p.decodeKey
		hsReq.AesIv = decodeIv
	}

	data, err := proto.Marshal(hsReq)
	if err != nil {
		return
	}

	if publicKey != nil {
		data, err = rsa.EncryptOAEP(sha1.New(), nil, publicKey, data, nil)
		if err != nil {
			return
		}
	}

	err = p.ws.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return
	}

	_, data, err = p.ws.ReadMessage()
	if err != nil {
		return
	}

	if p.decodeStream != nil {
		p.decodeStream.XORKeyStream(data, data)
	}

	hsRes := new(HandshakeRes)
	err = proto.Unmarshal(data, hsRes)
	if err != nil {
		return
	}

	if hsReq.GetChallenge() != hsRes.GetChallenge() {
		err = fmt.Errorf("challenge fail!")
		return
	}

	p.encodeKey = hsRes.GetAesKey()
	if p.encodeKey != nil {
		encodeIv := hsRes.GetAesIv()
		if len(encodeIv) != aes.BlockSize {
			err = fmt.Errorf("IV length must equal block size!")
			return
		}

		var encodeBlock cipher.Block
		encodeBlock, err = aes.NewCipher(p.encodeKey)
		if err != nil {
			return
		}

		p.encodeStream = cipher.NewCTR(encodeBlock, encodeIv)
	}

	return
}

func RsaPublicKeyFromPem(pemFilePath string) (*rsa.PublicKey, error) {
	f, err := os.Open(pemFilePath)
	if err != nil {
		return nil, err
	}

	fs, err := f.Stat()
	if err != nil {
		return nil, err
	}

	data := make([]byte, fs.Size())

	_, err = f.Read(data)
	if err != nil {
		return nil, err
	}

	pblock, _ := pem.Decode(data)
	if pblock == nil {
		return nil, fmt.Errorf("pem.Decode fail!")
	}

	var der []byte
	der = pblock.Bytes

	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}

	if pub == nil {
		return nil, fmt.Errorf("public key not found!")
	}

	key := pub.(*rsa.PublicKey)
	if key == nil {
		return nil, fmt.Errorf("key is not *rsa.PublicKey!")
	}

	return key, nil
}
