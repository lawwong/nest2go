package nest2go

import (
	. "github.com/lawwong/nest2go/protomsg"
	"github.com/lawwong/nest2go/uuid"
)

type AppListener interface {
	IncomingPeer(*PeerBase) PeerListener
	IncomingAgent(*AgentBase) AgentListener
	Teardown()
}

type AppSetupFunc func(*AppBase) AppListener

type AppSetuper interface {
	Setup(base *AppBase) AppListener
}

type AppBase struct {
	config   *Config_App
	server   *Server
	listener AppListener
}

func newAppBase(config *Config_App, server *Server) *AppBase {
	return &AppBase{
		config: config,
		server: server,
	}
}

func (a *AppBase) TypeName() string {
	return a.config.GetTypeName()
}

func (a *AppBase) Name() string {
	return a.config.GetIdName()
}

func (a *AppBase) SetupString() string {
	return a.config.GetSetupString()
}

func (a *AppBase) Server() *Server {
	return a.server
}

func (a *AppBase) newAgent(applyTime int64) *AgentBase {
	return nil
}

func (a *AppBase) verifyAgent(session uuid.Uuid, applyTime int64) (*AgentBase, error) {
	return nil, nil
}
