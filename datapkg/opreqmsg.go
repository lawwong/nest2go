package datapkg

import (
	"bytes"
	"fmt"
)

type ResponseSender interface {
	SendOpResponse(req OpRequest, resCode int32, data []byte) error
}

type OpRequest struct {
	code   int32
	serial int32
	data   []byte
	rs     ResponseSender
}

func MakeOpRequest(code int32, serial int32, data []byte, rs ResponseSender) OpRequest {
	return OpRequest{
		code:   code,
		serial: serial,
		data:   data,
		rs:     rs,
	}
}

func (m *OpRequest) Code() int32 {
	if m != nil {
		return m.code
	}
	return 0
}

func (m *OpRequest) Serial() int32 {
	if m != nil {
		return m.serial
	}
	return 0
}

func (m *OpRequest) Data() []byte {
	if m != nil {
		return m.data
	}
	return nil
}

func (m *OpRequest) SetResponseSender(rs ResponseSender) {
	if m != nil {
		m.rs = rs
	}
}

func (m *OpRequest) String() string {
	if m != nil {
		return fmt.Sprintf("OpRequest(%d)[%d]%d", m.code, m.serial, len(m.data))
	}
	return "OpRequest(nil)"
}

func (m *OpRequest) Respond(resCode int32, data []byte) error {
	if m == nil {
		return nil
	}
	if m.rs == nil {
		return nil
	}
	return m.rs.SendOpResponse(*m, resCode, data)
}

func (r1 OpRequest) IsEqual(r2 OpRequest) bool {
	if r1.code == r2.code &&
		r1.serial == r2.serial &&
		bytes.Equal(r1.data, r2.data) {
		return true
	}
	return false
}
