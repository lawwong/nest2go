// Code generated by protoc-gen-go.
// source: handshake.proto
// DO NOT EDIT!

package protomsg

import proto "github.com/golang/protobuf/proto"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = math.Inf

type HandshakeReq_Mode int32

const (
	HandshakeReq_PEER      HandshakeReq_Mode = 0
	HandshakeReq_NEW_AGENT HandshakeReq_Mode = 1
	HandshakeReq_AGENT     HandshakeReq_Mode = 2
)

var HandshakeReq_Mode_name = map[int32]string{
	0: "PEER",
	1: "NEW_AGENT",
	2: "AGENT",
}
var HandshakeReq_Mode_value = map[string]int32{
	"PEER":      0,
	"NEW_AGENT": 1,
	"AGENT":     2,
}

func (x HandshakeReq_Mode) Enum() *HandshakeReq_Mode {
	p := new(HandshakeReq_Mode)
	*p = x
	return p
}
func (x HandshakeReq_Mode) String() string {
	return proto.EnumName(HandshakeReq_Mode_name, int32(x))
}
func (x *HandshakeReq_Mode) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(HandshakeReq_Mode_value, data, "HandshakeReq_Mode")
	if err != nil {
		return err
	}
	*x = HandshakeReq_Mode(value)
	return nil
}

type HandshakeReq struct {
	Mode      *HandshakeReq_Mode `protobuf:"varint,1,req,name=mode,enum=protomsg.HandshakeReq_Mode" json:"mode,omitempty"`
	Challenge *int32             `protobuf:"varint,2,req,name=challenge" json:"challenge,omitempty"`
	Session   []byte             `protobuf:"bytes,3,opt,name=session" json:"session,omitempty"`
	// prevent replay attack, start with 0
	SerialNum *int32 `protobuf:"varint,4,opt,name=serial_num" json:"serial_num,omitempty"`
	// used when port rsa has set, AES CTR any key length
	AesKey           []byte `protobuf:"bytes,5,opt,name=aes_key" json:"aes_key,omitempty"`
	AesIv            []byte `protobuf:"bytes,6,opt,name=aes_iv" json:"aes_iv,omitempty"`
	XXX_unrecognized []byte `json:"-"`
}

func (m *HandshakeReq) Reset()         { *m = HandshakeReq{} }
func (m *HandshakeReq) String() string { return proto.CompactTextString(m) }
func (*HandshakeReq) ProtoMessage()    {}

func (m *HandshakeReq) GetMode() HandshakeReq_Mode {
	if m != nil && m.Mode != nil {
		return *m.Mode
	}
	return HandshakeReq_PEER
}

func (m *HandshakeReq) GetChallenge() int32 {
	if m != nil && m.Challenge != nil {
		return *m.Challenge
	}
	return 0
}

func (m *HandshakeReq) GetSession() []byte {
	if m != nil {
		return m.Session
	}
	return nil
}

func (m *HandshakeReq) GetSerialNum() int32 {
	if m != nil && m.SerialNum != nil {
		return *m.SerialNum
	}
	return 0
}

func (m *HandshakeReq) GetAesKey() []byte {
	if m != nil {
		return m.AesKey
	}
	return nil
}

func (m *HandshakeReq) GetAesIv() []byte {
	if m != nil {
		return m.AesIv
	}
	return nil
}

type HandshakeRes struct {
	Challenge *int32 `protobuf:"varint,1,req,name=challenge" json:"challenge,omitempty"`
	Session   []byte `protobuf:"bytes,2,opt,name=session" json:"session,omitempty"`
	// used when port rsa has set
	AesKey           []byte `protobuf:"bytes,3,opt,name=aes_key" json:"aes_key,omitempty"`
	AesIv            []byte `protobuf:"bytes,4,opt,name=aes_iv" json:"aes_iv,omitempty"`
	XXX_unrecognized []byte `json:"-"`
}

func (m *HandshakeRes) Reset()         { *m = HandshakeRes{} }
func (m *HandshakeRes) String() string { return proto.CompactTextString(m) }
func (*HandshakeRes) ProtoMessage()    {}

func (m *HandshakeRes) GetChallenge() int32 {
	if m != nil && m.Challenge != nil {
		return *m.Challenge
	}
	return 0
}

func (m *HandshakeRes) GetSession() []byte {
	if m != nil {
		return m.Session
	}
	return nil
}

func (m *HandshakeRes) GetAesKey() []byte {
	if m != nil {
		return m.AesKey
	}
	return nil
}

func (m *HandshakeRes) GetAesIv() []byte {
	if m != nil {
		return m.AesIv
	}
	return nil
}

func init() {
	proto.RegisterEnum("protomsg.HandshakeReq_Mode", HandshakeReq_Mode_name, HandshakeReq_Mode_value)
}
