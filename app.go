package nest2go

import (
	"fmt"
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/log"
	. "github.com/lawwong/nest2go/protomsg"
	"github.com/lawwong/nest2go/uuid"
	"path/filepath"
	"sync"
)

var (
	ErrAgentNotFound          = fmt.Errorf("agent not found")
	ErrInvalidHandshakeSerial = fmt.Errorf("invalid handshake serial number")
)

type AppBase struct {
	config *Config_App
	server *Server
	log    *FileLogger

	agentSetMut sync.RWMutex
	agentSet    map[uuid.Uuid]*AgentBase

	onpeer  chan *PeerBase
	onagent chan *AgentBase

	Closable
}

func newAppBase(server *Server, config *Config_App) *AppBase {
	a := &AppBase{
		config: config,
		server: server,
		log:    NewFileLogger(),

		onpeer:  make(chan *PeerBase),
		onagent: make(chan *AgentBase),

		Closable: MakeClosable(),
	}
	// FIXME : what if safeFileName returns empty?
	a.log.SetFilePath(filepath.Join(server.log.Dir(), "app"), safeFileName(config.GetName()))
	a.log.SetFileSepTime(server.log.Sep())
	// try add all ports
	if len(config.GetUsePorts()) > 0 {
		for _, addr := range config.GetUsePorts() {
			if p := server.findPort(addr); p != nil {
				p.addApp(a)
				a.log.Infof("listening port %q", addr)
			} else {
				a.log.Warnf("listening port %q fail! port not found", addr)
			}
		}
	}
	return a
}

func (a *AppBase) Log() *FileLogger {
	return a.log
}

func (a *AppBase) PeerChan() <-chan *PeerBase {
	return a.onpeer
}

func (a *AppBase) AgentChan() <-chan *AgentBase {
	return a.onagent
}

func (a *AppBase) AppType() string {
	return a.config.GetType()
}

func (a *AppBase) AppName() string {
	return a.config.GetName()
}

func (a *AppBase) SetupString() string {
	return a.config.GetSetupString()
}

func (a *AppBase) Server() *Server {
	return a.server
}

func (a *AppBase) newAgent(peer *PeerBase) *AgentBase {
	a.agentSetMut.Lock()
	defer a.agentSetMut.Unlock()

	var session *uuid.Uuid
	for {
		if session = uuid.New(); a.agentSet[*session] == nil {
			break
		}
	}

	agent := newAgentBase(session, a, peer)
	a.agentSet[*session] = agent

	go func(cloze <-chan struct{}) {
		<-cloze
		a.agentSetMut.Lock()
		defer a.agentSetMut.Unlock()
		delete(a.agentSet, *session)
	}(agent.CloseChan())

	return agent
}

func (a *AppBase) verifyAgent(session *uuid.Uuid, serial int32) (*AgentBase, error) {
	a.agentSetMut.RLock()
	defer a.agentSetMut.RUnlock()

	agent := a.agentSet[*session]
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	if serial <= agent.serial {
		return nil, ErrInvalidHandshakeSerial
	} else {
		agent.serial = serial
	}

	return agent, nil
}
