package nest2go

import (
	"github.com/lawwong/nest2go/uuid"
)

type AgentListener interface {
}

type AgentBase struct {
	session       uuid.Uuid
	lastApplyTime int64
	peer          *PeerBase
	listener      AgentListener
}

var _ PeerListener = (*AgentBase)(nil)

// return replaced peer(closed)
func (a *AgentBase) setPeer(p *PeerBase) (replaced *PeerBase) {
	if a.peer != nil {
		replaced = a.peer
		replaced.Close()
	}
	a.peer = p
	return
	// TODO : test p is nil?
}
