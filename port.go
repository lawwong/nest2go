package nest2go

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gorilla/websocket"
	. "github.com/lawwong/nest2go/closable"
	. "github.com/lawwong/nest2go/log"
	. "github.com/lawwong/nest2go/protomsg"
	"github.com/stretchr/graceful"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var appNameReg = regexp.MustCompile("^/([^/]*)")

var wsUpgrader = &websocket.Upgrader{
	//ReadBufferSize:  1024,
	//WriteBufferSize: 1024,
	CheckOrigin: func(*http.Request) bool { return true },
}

type port struct {
	//server *Server
	config *Config_Port
	log    *FileLogger

	appSetMut sync.RWMutex
	appSet    map[string]*AppBase

	rsaPK    *rsa.PrivateKey
	wsServer *graceful.Server

	Closable
}

func newPort(s *Server, config *Config_Port) (p *port, err error) {
	// load rsa private key if pem file name is set
	var pk *rsa.PrivateKey
	if fname := config.GetRsaKeyPem(); fname != "" {
		if pk, err = LoadPrivateKeyFromFile(fname, s.rsaPwd); err != nil {
			return
		}
	}

	log := NewFileLogger()
	// FIXME : what if safeFileName returns empty?
	log.SetFilePath(filepath.Join(s.log.Dir(), "port"), safeFileName(config.GetAddr()))
	log.SetFileSepTime(s.log.Sep())

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.wsHandler)

	p = &port{
		appSet: make(map[string]*AppBase),
		rsaPK:  pk,
		config: config,
		//server: s,
		wsServer: &graceful.Server{
			Server: &http.Server{
				Addr:    config.GetAddr(),
				Handler: mux,
			},
		},
		Closable: MakeClosable(),
	}
	p.HandleClose(nil)
	go p.runServer()
	return p, nil
}

func (p *port) runServer() {
	var err error
	cert := p.config.GetTlsCertFile()
	key := p.config.GetTlsKeyFile()
	if cert == "" && key == "" {
		p.log.Info("start serving")
		err = p.wsServer.ListenAndServe()
		p.log.Info("end serving : ", err)
	} else {
		p.log.Info("start serving tls")
		err = p.wsServer.ListenAndServeTLS(cert, key)
		p.log.Info("end serving tls : ", err)
	}

	p.Close()
}

func (p *port) wsHandler(w http.ResponseWriter, req *http.Request) {
	var err error
	var appName string
	defer printErr(p.log, &err, "port.wsHandler", req.RemoteAddr, appName)

	uri, _ := url.QueryUnescape(req.URL.RequestURI())
	subMatch := appNameReg.FindStringSubmatch(uri)
	if len(subMatch) < 2 {
		err = fmt.Errorf("Invalid URI(%q)!", uri)
		return
	}

	appName = subMatch[1]
	app := p.findApp(appName)
	if app == nil {
		err = ErrAppNotFound
		return
	}

	ws, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}

	newPeerBase(ws, app, p.rsaPK)
}

func (p *port) addApp(app *AppBase) {
	p.appSetMut.Lock()
	defer p.appSetMut.Unlock()
	if app != nil {
		p.appSet[app.AppName()] = app
	}
}

func (p *port) deleteApp(appName string) {
	p.appSetMut.Lock()
	defer p.appSetMut.Unlock()
	delete(p.appSet, appName)
}

func (p *port) findApp(appName string) *AppBase {
	p.appSetMut.RLock()
	defer p.appSetMut.RUnlock()
	return p.appSet[appName]
}

func (p *port) HandleClose(handler CloseHandler) {
	p.Closable.HandleCloseFunc(func() {
		p.wsServer.Stop(0)
		<-p.wsServer.StopChan()
		if handler != nil {
			handler.HandleClose()
		}
	})
}

func (p *port) HandleCloseFunc(handleFunc CloseHandlerFunc) {
	p.HandleClose(CloseHandler(handleFunc))
}

func LoadPrivateKeyFromFile(filename string, pwd string) (pk *rsa.PrivateKey, err error) {
	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	var fs os.FileInfo
	fs, err = f.Stat()
	if err != nil {
		return
	}

	data := make([]byte, fs.Size())
	_, err = f.Read(data)
	if err != nil {
		return
	}

	pblock, _ := pem.Decode(data)
	if pblock == nil {
		err = fmt.Errorf("pem.Decode fail!")
		return
	}

	var der []byte
	if len(pblock.Headers) == 0 || pwd == "" {
		// no header found! used as plain der
		der = pblock.Bytes
	} else {
		der, err = x509.DecryptPEMBlock(pblock, []byte(pwd))
		if err != nil {
			return
		}
	}

	return x509.ParsePKCS1PrivateKey(der)
}
