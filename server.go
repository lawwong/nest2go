package nest2go

import (
	"github.com/golang/protobuf/proto"
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/log"
	. "github.com/lawwong/nest2go/protomsg"
	"os"
	"sync"
)

type AppHandler interface {
	HandleApp(app *AppBase)
}

type AppHandlerFunc func(app *AppBase)

func (h AppHandlerFunc) HandleApp(app *AppBase) {
	h(app)
}

type Server struct {
	log    *FileLogger
	rsaPwd string

	handleSetMut sync.RWMutex
	handleSet    map[string]AppHandler

	portSetMut sync.RWMutex
	portSet    map[string]*port

	appSetMut sync.RWMutex
	appSet    map[string]*AppBase

	Closable
}

func NewServer(rsaKeyPassword string) *Server {
	s := &Server{
		log:    NewFileLogger(),
		rsaPwd: rsaKeyPassword,

		handleSet: make(map[string]AppHandler),
		portSet:   make(map[string]*port),
		appSet:    make(map[string]*AppBase),

		Closable: MakeClosable(),
	}
	s.HandleClose(nil)
	return s
}

func (s *Server) Log() *FileLogger {
	return s.log
}

func (s *Server) HandleAppType(appType string, handler AppHandler) {
	s.RLockClosedMut()
	defer s.RUnlockClosedMut()
	if s.Closed() {
		panic("nest2go: server closed")
	}

	if appType == "" {
		panic("nest2go: invalid typeName " + appType)
	}

	if handler == nil {
		panic("nest2go: nil handler")
	}

	if !s.registorHandler(appType, handler) {
		panic("nest2go: multiple registrations for " + appType)
	}
}

func (s *Server) HadleAppTypFunc(appType string, handleFunc AppHandlerFunc) {
	s.HandleAppType(appType, AppHandler(handleFunc))
}

func (s *Server) OpenPort(portConf *Config_Port) (err error) {
	addr := portConf.GetAddr()
	defer printErr(s.log, &err, "OpenPort", addr)

	s.RLockClosedMut()
	defer s.RUnlockClosedMut()
	if s.Closed() {
		return ErrServiceClosed
	}

	if addr == "" {
		return ErrInvalidArgument
	}

	s.portSetMut.Lock()
	defer s.portSetMut.Unlock()
	if s.portSet[addr] != nil {
		return ErrMultipleRegistrations
	}

	p, err := newPort(s, portConf)
	if err != nil {
		return err
	}

	s.portSet[addr] = p
	// remove port from portSet after port closed
	go s.runOnPortClosed(p)

	return nil
}

// remove port from portSet after port closed
func (s *Server) runOnPortClosed(p *port) {
	<-p.CloseChan()
	s.portSetMut.Lock()
	defer s.portSetMut.Unlock()
	addr := p.config.GetAddr()
	if s.portSet[addr] == p {
		delete(s.portSet, addr)
	}
	s.log.Infof("port %q closed", p.config.GetAddr())
}

// return nil if target port not found
func (s *Server) ClosePort(addr string) error {
	s.portSetMut.RLock()
	defer s.portSetMut.RUnlock()

	if p := s.portSet[addr]; p == nil {
		return ErrPortNotFound
	} else {
		p.Close()
		return nil
	}
}

func (s *Server) CloseAllPorts() {
	s.portSetMut.RLock()
	defer s.portSetMut.RUnlock()

	for _, p := range s.portSet {
		p.Close()
	}
}

func (s *Server) OpenApp(appConf *Config_App) (err error) {
	appType := appConf.GetType()
	appName := appConf.GetName()
	defer printErr(s.log, &err, "OpenApp", appType, appName)

	s.RLockClosedMut()
	defer s.RUnlockClosedMut()
	if s.Closed() {
		return ErrServiceClosed
	}

	if appType == "" || appName == "" {
		return ErrInvalidArgument
	}

	handler := s.findHandler(appType)
	if handler == nil {
		return ErrHandlerNotFound
	}

	s.appSetMut.Lock()
	defer s.appSetMut.Unlock()
	if s.appSet[appName] != nil {
		return ErrMultipleRegistrations
	}
	app := newAppBase(s, appConf)
	s.appSet[appName] = app
	// send app to app handler
	go handler.HandleApp(app)
	// remove app from appSet after app closed
	go s.runOnAppClosed(app)

	return nil
}

// remove app from appSet after app closed
func (s *Server) runOnAppClosed(a *AppBase) {
	<-a.CloseChan()
	s.appSetMut.Lock()
	defer s.appSetMut.Unlock()
	appName := a.config.GetName()
	if s.appSet[appName] == a {
		delete(s.appSet, appName)
	}
	s.log.Infof("app %q(%q) closed", a.config.GetName(), a.config.GetType())
}

func (s *Server) CloseApp(appName string) error {
	s.appSetMut.RLock()
	defer s.appSetMut.RUnlock()

	if app := s.appSet[appName]; app == nil {
		return ErrAppNotFound
	} else {
		app.Close()
		return nil
	}
}

func (s *Server) CloseAllApps() {
	s.appSetMut.RLock()
	defer s.appSetMut.RUnlock()

	for _, app := range s.appSet {
		app.Close()
	}
}

func (s *Server) LoadConfigFile(filePath string) (err error) {
	defer printErr(s.log, &err, "LoadConfigFile", filePath)

	f, err := os.Open(filePath)
	if err != nil {
		return
	}

	defer f.Close()

	fs, err := f.Stat()
	if err != nil {
		return
	}

	data := make([]byte, fs.Size())
	config := new(Config)
	f.Read(data)
	err = proto.UnmarshalText(string(data), config)
	if err != nil {
		return
	}

	s.LoadConfig(config)
	return nil
}

func (s *Server) LoadConfig(config *Config) {
	var err error
	defer printErr(s.log, &err, "LoadConfig")

	s.RLockClosedMut()
	defer s.RUnlockClosedMut()
	if s.Closed() {
		err = ErrServiceClosed
		return
	}
	// log
	s.log.SetFilePath(config.GetServerName(), config.GetLogDir())
	s.log.SetFileSepTime(config.GetLogSepTime())
	// load all ports
	for _, portConf := range config.GetPorts() {
		s.OpenPort(portConf)
	}
	// load all apps
	for _, appConf := range config.GetApps() {
		s.OpenApp(appConf)
	}
}

func (s *Server) CurrentConfig() *Config {
	s.RLockClosedMut()
	defer s.RUnlockClosedMut()

	s.portSetMut.RLock()
	var ports []*Config_Port
	if portCount := len(s.portSet); portCount > 0 {
		ports = make([]*Config_Port, portCount)
		i := 0
		for _, p := range s.portSet {
			ports[i] = proto.Clone(p.config).(*Config_Port)
			i++
		}
	}
	s.portSetMut.RUnlock()

	s.appSetMut.RLock()
	var apps []*Config_App
	if appCount := len(s.appSet); appCount > 0 {
		apps = make([]*Config_App, appCount)
		i := 0
		for _, app := range s.appSet {
			apps[i] = proto.Clone(app.config).(*Config_App)
			i++
		}
	}
	s.appSetMut.RUnlock()

	return &Config{
		ServerName: proto.String(s.log.Name()),
		LogDir:     proto.String(s.log.Dir()),
		LogSepTime: s.log.Sep().Enum(),
		Ports:      ports,
		Apps:       apps,
	}
}

func (s *Server) SaveCurrentConfigFile(filePath string) (err error) {
	defer printErr(s.log, &err, "SaveCurrentConfigFile", filePath)

	s.RLockClosedMut()
	defer s.RUnlockClosedMut()
	if s.Closed() {
		return ErrServiceClosed
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0664)
	if err != nil {
		return
	}
	defer f.Close()

	err = proto.MarshalText(f, s.CurrentConfig())
	return
}

func (s *Server) registorHandler(appType string, handler AppHandler) bool {
	s.handleSetMut.Lock()
	defer s.handleSetMut.Unlock()
	if s.handleSet[appType] != nil {
		return false
	}
	s.handleSet[appType] = handler
	return true
}

func (s *Server) findHandler(appType string) AppHandler {
	s.handleSetMut.RLock()
	defer s.handleSetMut.RUnlock()
	return s.handleSet[appType]
}

func (s *Server) findPort(addr string) *port {
	s.portSetMut.RLock()
	defer s.portSetMut.RUnlock()
	return s.portSet[addr]
}

func (s *Server) findApp(appName string) *AppBase {
	s.appSetMut.RLock()
	defer s.appSetMut.RUnlock()
	return s.appSet[appName]
}

func (s *Server) onClose() {
	var wg sync.WaitGroup
	// close all ports
	s.portSetMut.RLock()
	wg.Add(len(s.portSet))
	for _, p := range s.portSet {
		go func(p *port) {
			p.Close()
			<-p.CloseChan()
			wg.Done()
		}(p)
	}
	s.portSetMut.RUnlock()
	wg.Wait()
	// close all apps
	s.appSetMut.RLock()
	wg.Add(len(s.appSet))
	for _, app := range s.appSet {
		go func(a *AppBase) {
			a.Close()
			<-a.CloseChan()
			wg.Done()
		}(app)
	}
	s.appSetMut.RUnlock()
	wg.Wait()
}

func (s *Server) HandleClose(handler CloseHandler) {
	s.Closable.HandleCloseFunc(func() {
		s.onClose()
		if handler != nil {
			handler.HandleClose()
		}
	})
}

func (s *Server) HandleCloseFunc(handleFunc CloseHandlerFunc) {
	s.HandleClose(CloseHandler(handleFunc))
}
