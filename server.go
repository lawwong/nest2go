package nest2go

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	. "github.com/lawwong/nest2go/protomsg"
	"io"
	"os"
	"strings"
	"sync"
)

type SIG_TYPE struct{}

var SIGNAL = SIG_TYPE{}

type Setuper interface {
	Setup(base *AppBase) AppListener
}

type SetuperFunc func(*AppBase) AppListener

func (f SetuperFunc) Setup(a *AppBase) AppListener {
	return f(a)
}

type addSetuperInfo struct {
	typeName string
	setuper  Setuper
}

type setupAppInfo struct {
	err    chan error
	config *Config_App
}

type teardownAppInfo struct {
	err  chan error
	name string
}

type closePortInfo struct {
	err  chan error
	addr string
}

type alterAppPortInfo struct {
	appName string
	addr    string
}

type loadOp int

const (
	loadOpNone loadOp = iota
	loadOpStart
	loadOpReload
	loadOpStop
)

type Server struct {
	config *Config
	log    *FileLogger
	rsaPwd string

	setuperSet map[string]Setuper
	portSet    map[string]*port
	appSet     map[string]*AppBase

	addSetuper chan addSetuperInfo

	unmarshalConf chan string
	marshalConf   chan io.Writer
	cloneConf     chan chan *Config
	updateConf    chan SIG_TYPE
	setConf       chan *Config

	setupApp       chan setupAppInfo
	addApp         chan *AppBase
	teardownApp    chan teardownAppInfo
	setupAllApp    chan *sync.WaitGroup
	teardownAllApp chan *sync.WaitGroup

	openPort     chan *Config_Port
	closePort    chan closePortInfo
	delPort      chan string
	openAllPort  chan *sync.WaitGroup
	closeAllPort chan *sync.WaitGroup
	addAppPort   chan alterAppPortInfo
	delAppPort   chan alterAppPortInfo

	loadLog chan SIG_TYPE
	setPwd  chan string

	err     chan error
	stop    chan struct{}
	serving chan struct{}

	isServingMut sync.Mutex
	isServing    bool

	add  chan SIG_TYPE
	done chan SIG_TYPE
	lock chan chan chan SIG_TYPE
}

func NewServer() *Server {
	s := &Server{
		config: new(Config),
		log:    NewFileLogger(),

		setuperSet: make(map[string]Setuper),
		portSet:    make(map[string]*port),
		appSet:     make(map[string]*AppBase),

		addSetuper: make(chan addSetuperInfo),

		unmarshalConf: make(chan string),
		marshalConf:   make(chan io.Writer),
		cloneConf:     make(chan chan *Config),
		updateConf:    make(chan SIG_TYPE),
		setConf:       make(chan *Config),

		setupApp:       make(chan setupAppInfo),
		addApp:         make(chan *AppBase),
		teardownApp:    make(chan teardownAppInfo),
		setupAllApp:    make(chan *sync.WaitGroup),
		teardownAllApp: make(chan *sync.WaitGroup),

		openPort:     make(chan *Config_Port),
		closePort:    make(chan closePortInfo),
		delPort:      make(chan string),
		openAllPort:  make(chan *sync.WaitGroup),
		closeAllPort: make(chan *sync.WaitGroup),
		addAppPort:   make(chan alterAppPortInfo),
		delAppPort:   make(chan alterAppPortInfo),

		loadLog: make(chan SIG_TYPE),
		setPwd:  make(chan string),

		err:     make(chan error),
		stop:    make(chan struct{}),
		serving: make(chan struct{}),

		add:  make(chan SIG_TYPE),
		done: make(chan SIG_TYPE),
		lock: make(chan chan chan SIG_TYPE),
	}
	go s.run()
	go s.runOpLock()
	return s
}

func (s *Server) Log() Logger {
	return s.log
}

// prevent from deadlock
func (s *Server) runOpLock() {
	readers := 0
	for {
		select {
		case <-s.add:
			readers++
		case <-s.done:
			readers--
		case unlockChan := <-s.lock:
			if readers > 0 {
				unlockChan <- nil
			} else {
				unlocker := make(chan SIG_TYPE)
				unlockChan <- unlocker
				<-unlocker
			}
		case <-s.stop:
			break
		}
	}
}

func (s *Server) run() {
	for {
		select {
		case info := <-s.addSetuper:
			if _, found := s.setuperSet[info.typeName]; found {
				s.err <- fmt.Errorf("Multiple setuper")
			} else {
				s.setuperSet[info.typeName] = info.setuper
				s.err <- nil
			}
		case dataText := <-s.unmarshalConf:
			s.err <- proto.UnmarshalText(dataText, s.config)
		case w := <-s.marshalConf:
			s.err <- proto.MarshalText(w, s.config)
		case confChan := <-s.cloneConf:
			confChan <- proto.Clone(s.config).(*Config)
		case <-s.updateConf:
			// update current port configs
			i := 0
			s.config.OpenPorts = make([]*Config_Port, len(s.portSet))
			for _, p := range s.portSet {
				s.config.OpenPorts[i] = p.config
				i++
			}
			// update current app configs
			i = 0
			s.config.Apps = make([]*Config_App, len(s.appSet))
			for _, a := range s.appSet {
				s.config.Apps[i] = a.config
				i++
			}

			s.err <- nil
		case conf := <-s.setConf:
			if conf == nil {
				s.config = new(Config)
			} else {
				s.config = conf
			}
			s.err <- nil
		case info := <-s.setupApp:
			typeName := info.config.GetTypeName()
			idName := info.config.GetIdName()
			setuper := s.setuperSet[typeName]
			if setuper == nil {
				info.err <- fmt.Errorf("App setuper not found!")
				continue
			}
			if _, found := s.appSet[idName]; found {
				info.err <- fmt.Errorf("Multiple app")
				continue
			}

			s.appSet[idName] = nil

			go func(setuper Setuper, info setupAppInfo) {
				app := newAppBase(info.config, s)
				app.listener = setuper.Setup(app)
				select {
				case <-s.stop:
					if app.listener != nil {
						app.listener.Teardown()
					}
					info.err <- fmt.Errorf("Server closed after setup!")
				case s.addApp <- app:
					info.err <- <-s.err
				}
			}(setuper, info)
		case app := <-s.addApp:
			// set app listen ports
			for _, portAddr := range app.config.GetListenPortAddr() {
				if p, found := s.portSet[portAddr]; found {
					p.addApp(app)
				} else {
					s.log.Warnf("Listen port addr(%q) for app(%q) not found!", portAddr, app.config.GetIdName())
				}
			}

			s.appSet[app.config.GetIdName()] = app
			s.err <- nil
		case info := <-s.teardownApp:
			app := s.appSet[info.name]
			if app == nil {
				info.err <- fmt.Errorf("Target app not found!")
				continue
			}
			// remove listen ports
			for _, p := range s.portSet {
				p.delApp(info.name)
			}
			// TODO : disconnect all peer and agent?
			delete(s.appSet, info.name)
			go func(app *AppBase, info teardownAppInfo) {
				app.listener.Teardown()
				info.err <- nil
			}(app, info)
		case wg := <-s.setupAllApp:
			appConfigs := s.config.GetApps()
			if appConfigLen := len(appConfigs); appConfigLen > 0 {
				wg.Add(appConfigLen)
				for _, appConfig := range appConfigs {
					go func(appConfig *Config_App, wg *sync.WaitGroup) {
						s._SetupApp(appConfig)
						wg.Done()
					}(appConfig, wg)
				}
			}
			wg.Done()
		case wg := <-s.teardownAllApp:
			appSetLen := len(s.appSet)
			if appSetLen > 0 {
				wg.Add(appSetLen)
				for appName := range s.appSet {
					go func(appName string, wg *sync.WaitGroup) {
						s._TeardownApp(appName)
						wg.Done()
					}(appName, wg)
				}
			}
			wg.Done()
		case portConfig := <-s.openPort:
			addr := portConfig.GetAddr()
			if _, found := s.portSet[addr]; found {
				s.err <- fmt.Errorf("Multiple port addr")
				continue
			}

			p, err := newPort(portConfig, s)
			if err != nil {
				s.err <- err
				continue
			}

			s.portSet[addr] = p

			go func(p *port) {
				if err := p.ListenAndServe(); err != nil {
					s.log.Info(err)
				}

				s.delPort <- p.config.GetAddr()
				<-s.err
			}(p)

			s.err <- nil
		case info := <-s.closePort:
			p := s.portSet[info.addr]
			if p == nil {
				s.err <- fmt.Errorf("Target port not found!")
				continue
			}

			p.Stop(0)
			go func(p *port, info closePortInfo) {
				<-p.StopChan()
				s.delPort <- info.addr
				info.err <- <-s.err
			}(p, info)
		case addr := <-s.delPort:
			delete(s.portSet, addr)
			s.err <- nil
		case wg := <-s.openAllPort:
			portConfigs := s.config.GetOpenPorts()
			if portConfigLen := len(portConfigs); portConfigLen > 0 {
				wg.Add(portConfigLen)
				for _, portConfig := range portConfigs {
					go func(portConfig *Config_Port, wg *sync.WaitGroup) {
						s._OpenPort(portConfig)
						wg.Done()
					}(portConfig, wg)
				}
			}
			wg.Done()
		case wg := <-s.closeAllPort:
			portSetLen := len(s.portSet)
			if portSetLen > 0 {
				wg.Add(portSetLen)
				for addr := range s.portSet {
					go func(addr string, wg *sync.WaitGroup) {
						s._ClosePort(addr)
						wg.Done()
					}(addr, wg)
				}
			}
			wg.Done()
		case info := <-s.addAppPort:
			app := s.appSet[info.appName]
			if app == nil {
				s.err <- fmt.Errorf("App not found!")
				continue
			}

			p := s.portSet[info.addr]
			if p == nil {
				s.err <- fmt.Errorf("Port not found!")
			}

			p.addApp(app)
			s.err <- nil
		case info := <-s.delAppPort:
			app := s.appSet[info.appName]
			if app == nil {
				s.err <- fmt.Errorf("App not found!")
				continue
			}

			p := s.portSet[info.addr]
			if p == nil {
				s.err <- fmt.Errorf("Port not found!")
			}

			p.delApp(info.appName)
			s.err <- nil
		case <-s.loadLog:
			s.log.SetFilePath(s.config.GetLogDir(), s.config.GetServerName(), FileSepDurDay)
			s.err <- nil
		case pwd := <-s.setPwd:
			s.rsaPwd = pwd
			s.err <- nil
		case <-s.stop:
			break
		}
	}
}

func (s *Server) HandleSetuper(typeName string, setuper Setuper) (err error) {
	defer s.printErr(&err, "HandleSetuper", typeName)

	if typeName == "" {
		err = fmt.Errorf("Empty typeName!")
		return
	}

	if setuper == nil {
		err = fmt.Errorf("nil setuper!")
		return
	}

	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.addSetuper <- addSetuperInfo{typeName, setuper}:
		err = <-s.err
	}
	return
}

func (s *Server) HandleSetuperFunc(n string, f SetuperFunc) error {
	return s.HandleSetuper(n, SetuperFunc(f))
}

func (s *Server) LoadConfigFile(filePath string) error {
	return s.lockOp(
		func() error { return s._LoadConfigFile(filePath) },
		"LoadConfig",
		filePath,
	)
}

func (s *Server) _LoadConfigFile(filePath string) (err error) {
	defer s.printErr(&err, "LoadConfig", filePath)

	//f, err := os.OpenFile(filePath, os.O_RDONLY|os.O_CREATE, 0664)
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
	_, err = f.Read(data)
	if err != nil {
		return
	}

	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.unmarshalConf <- string(data):
		err = <-s.err
	}
	return
}

func (s *Server) SaveConfigFile(filePath string) error {
	return s.lockOp(
		func() error { return s._SaveConfigFile(filePath) },
		"SaveConfig",
		filePath,
	)
}

func (s *Server) _SaveConfigFile(filePath string) (err error) {
	defer s.printErr(&err, "SaveConfig", filePath)

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0664)
	if err != nil {
		return
	}
	defer f.Close()

	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.marshalConf <- f:
		err = <-s.err
	}
	return
}

func (s *Server) Config() *Config {
	confChan := make(chan *Config)
	select {
	case s.cloneConf <- confChan:
		return <-confChan
	case <-s.stop:
		return proto.Clone(s.config).(*Config)
	}
}

func (s *Server) UpdateConfig() (config *Config, err error) {
	s.lockOp(
		func() error {
			config, err = s._UpdateConfig()
			return err
		},
		"UpdateConfig",
	)
	return
}

func (s *Server) _UpdateConfig() (config *Config, err error) {
	defer s.printErr(&err, "UpdateConfig")

	select {
	case s.updateConf <- SIGNAL:
		err = <-s.err
		config = s.Config()
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	}
	return
}

func (s *Server) SetConfig(config *Config) (err error) {
	return s.lockOp(
		func() error { return s._SetConfig(config) },
		"SetConfig",
	)
}

func (s *Server) _SetConfig(config *Config) (err error) {
	defer s.printErr(&err, "SetConfig")

	select {
	case s.setConf <- config:
		err = <-s.err
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	}
	return
}

func (s *Server) SetupApp(appConfig *Config_App) error {
	return s.lockOp(
		func() error { return s._SetupApp(appConfig) },
		"SetupApp",
		appConfig.GetTypeName(),
		appConfig.GetIdName(),
	)
}

func (s *Server) _SetupApp(appConfig *Config_App) (err error) {
	defer s.printErr(&err, "SetupApp", appConfig.GetTypeName(), appConfig.GetIdName())

	if appConfig == nil {
		err = fmt.Errorf("nil appConfig!")
		return
	}

	errChan := make(chan error)
	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.setupApp <- setupAppInfo{errChan, appConfig}:
		err = <-errChan
	}
	return
}

func (s *Server) SetupAllApp() {
	s.lockOp(
		func() error {
			s._SetupAllApp()
			return nil
		},
		"SetupAllApp",
	)
}

func (s *Server) _SetupAllApp() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.setupAllApp <- wg
	wg.Wait()
}

func (s *Server) TeardownApp(appName string) error {
	return s.lockOp(
		func() error { return s._TeardownApp(appName) },
		"TeardownApp",
		appName,
	)
}

func (s *Server) _TeardownApp(appName string) (err error) {
	defer s.printErr(&err, "TeardownApp", appName)

	errChan := make(chan error)
	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.teardownApp <- teardownAppInfo{errChan, appName}:
		err = <-errChan
	}
	return
}

func (s *Server) TeardownAllApp() {
	s.lockOp(
		func() error {
			s._TeardownAllApp()
			return nil
		},
		"TeardownAllApp",
	)
}

func (s *Server) _TeardownAllApp() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.teardownAllApp <- wg
	wg.Wait()
}

func (s *Server) OpenPort(portConfig *Config_Port) error {
	return s.lockOp(
		func() error { return s._OpenPort(portConfig) },
		"OpenPort",
		portConfig.GetAddr(),
	)
}

func (s *Server) _OpenPort(portConfig *Config_Port) (err error) {
	defer s.printErr(&err, "OpenPort", portConfig.GetAddr())

	if portConfig == nil {
		err = fmt.Errorf("nil portConfig!")
		return
	}

	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.openPort <- portConfig:
		err = <-s.err
	}
	return
}

func (s *Server) OpenAllPort() {
	s.lockOp(
		func() error {
			s._OpenAllPort()
			return nil
		},
		"OpenAllPort",
	)
}

func (s *Server) _OpenAllPort() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.openAllPort <- wg
	wg.Wait()
}

func (s *Server) ClosePort(addr string) error {
	return s.lockOp(
		func() error { return s._ClosePort(addr) },
		"ClosePort",
		addr,
	)
}

func (s *Server) _ClosePort(addr string) (err error) {
	errChan := make(chan error)
	select {
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	case s.closePort <- closePortInfo{errChan, addr}:
		err = <-errChan
	}
	if err != nil {
		s.log.Warnf("ClosePort(%q) fail! %s", addr, err)
	} else {
		s.log.Infof("ClosePort(%q) ok!", addr)
	}
	return
}

func (s *Server) CloseAllPort() {
	s.lockOp(
		func() error {
			s._CloseAllPort()
			return nil
		},
		"CloseAllPort",
	)
}

func (s *Server) _CloseAllPort() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	s.closeAllPort <- wg
	wg.Wait()
}

func (s *Server) SetRsaPassword(pwd string) {
	select {
	case s.setPwd <- pwd:
		<-s.err
	case <-s.stop:
	}
}

func (s *Server) unload() {
	// close apps listening ports
	s._CloseAllPort()
	// tear down apps
	s._TeardownAllApp()
}

func (s *Server) load() {
	// load server logger
	s.loadLog <- SIGNAL
	<-s.err
	// setup app
	s._SetupAllApp()
	// listen apps ports
	s._OpenAllPort()
}

func (s *Server) Serve() (err error) {
	defer s.printErr(&err, "Serve")
	// prevent from duplicate Serve() call
	s.isServingMut.Lock()
	if s.isServing {
		s.isServingMut.Unlock()
		err = fmt.Errorf("Server is already running!")
		return
	}

	s.isServing = true
	s.isServingMut.Unlock()
	defer func() {
		if err != nil {
			s.isServingMut.Lock()
			s.isServing = false
			s.isServingMut.Unlock()
		}
	}()

	unlocker, err := s.lockLoadOp()
	if err != nil {
		return
	}

	s.load()
	unlocker <- SIGNAL
	return
}

func (s *Server) Reload() (err error) {
	defer s.printErr(&err, "Reload")

	unlocker, err := s.lockLoadOp()
	if err != nil {
		return
	}

	s.unload()
	s.load()
	unlocker <- SIGNAL
	return
}

func (s *Server) Stop() (err error) {
	defer s.printErr(&err, "Stop")

	unlocker, err := s.lockLoadOp()
	if err != nil {
		return
	}

	s.unload()
	close(s.stop)
	unlocker <- SIGNAL
	return
}

func (s *Server) lockOp(f func() error, fName string, args ...interface{}) (err error) {
	select {
	case s.add <- SIGNAL:
		err = f()
		s.done <- SIGNAL
		return
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	default:
		err = fmt.Errorf("Server is locked by load/reload/stop!")
	}
	s.printErr(&err, fName, args...)
	return
}

func (s *Server) lockLoadOp() (unlocker chan SIG_TYPE, err error) {
	unlockerChan := make(chan chan SIG_TYPE)
	select {
	case s.lock <- unlockerChan:
		unlocker = <-unlockerChan
		if unlocker == nil {
			err = fmt.Errorf("Server is locked by other op!")
		}
	case <-s.stop:
		err = fmt.Errorf("Server closed!")
	default:
		err = fmt.Errorf("Server is locked by load/reload/stop!")
	}
	return
}

func (s *Server) printErr(err *error, funcName string, args ...interface{}) {
	argsStr := fmt.Sprintf(strings.Repeat("%v,", len(args)), args...)
	if err != nil {
		s.log.Warnf("%s(%s) fail! %v", funcName, argsStr, *err)
	} else {
		s.log.Infof("%s(%s) ok!", funcName, argsStr)
	}
}
