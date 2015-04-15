package nest2go

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gorilla/websocket"
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
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type port struct {
	//server *Server
	config *Config_Port
	log    *FileLogger

	appSetMut sync.RWMutex
	appSet    map[string]*AppBase

	rsaPK    *rsa.PrivateKey
	wsServer *graceful.Server

	*closable
}

func newPort(s *Server, config *Config_Port) (p *port, err error) {
	// load rsa private key if pem file name is set
	var pk *rsa.PrivateKey
	if fname := config.GetRsaKeyPem(); fname != "" {
		var f *os.File
		f, err = os.Open(fname)
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
		if len(pblock.Headers) == 0 || s.rsaPwd == "" {
			// no header found! used as plain der
			der = pblock.Bytes
		} else {
			der, err = x509.DecryptPEMBlock(pblock, []byte(s.rsaPwd))
			if err != nil {
				return nil, err
			}
		}

		pk, err = x509.ParsePKCS1PrivateKey(der)
		if err != nil {
			return nil, err
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
		closable: newClosable(),
	}
	p.HandleCloseFunc(func() {})
	go p.runServer()
	return p, nil
}

func (p *port) Log() *FileLogger {
	return p.log
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
	p.closable.HandleCloseFunc(func() {
		p.wsServer.Stop(0)
		handler.HandleClose()
	})
}
