package datapkg

import (
	"fmt"
)

type EventData struct {
	code int32
	data []byte
}

func MakeEventData(code int32, data []byte) EventData {
	return EventData{
		code: code,
		data: data,
	}
}

func (m *EventData) Code() int32 {
	if m != nil {
		return m.code
	}
	return 0
}

func (m *EventData) Data() []byte {
	if m != nil {
		return m.data
	}
	return nil
}

func (m *EventData) String() string {
	if m != nil {
		return fmt.Sprintf("EventData(%d)%d", m.code, len(m.data))
	}
	return "EventData(nil)"
}
