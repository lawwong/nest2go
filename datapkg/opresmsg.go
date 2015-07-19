package datapkg

import (
	"fmt"
)

type OpResponse struct {
	code    int32
	resCode int32
	serial  int32
	data    []byte
}

func MakeOpResponse(code int32, resCode int32, serial int32, data []byte) OpResponse {
	return OpResponse{
		code:    code,
		resCode: resCode,
		serial:  serial,
		data:    data,
	}
}

func (m *OpResponse) Code() int32 {
	if m != nil {
		return m.code
	}
	return 0
}

func (m *OpResponse) ResponseCode() int32 {
	if m != nil {
		return m.resCode
	}
	return 0
}

func (m *OpResponse) Serial() int32 {
	if m != nil {
		return m.serial
	}
	return 0
}

func (m *OpResponse) Data() []byte {
	if m != nil {
		return m.data
	}
	return nil
}

func (m *OpResponse) String() string {
	if m != nil {
		return fmt.Sprintf("OpResponse(%d,%d)[%d]%d", m.code, m.resCode, m.serial, len(m.data))
	}
	return "OpResponse(nil)"
}
