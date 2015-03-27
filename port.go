package nest2go

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gorilla/websocket"
	. "github.com/lawwong/nest2go/protomsg"
	"github.com/stretchr/graceful"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sync"
)

var appNameReg = regexp.MustCompile("^/([^/]*)")

var wsUpgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type port struct {
	appSetMut sync.RWMutex
	appSet    map[string]*AppBase
	rsaPK     *rsa.PrivateKey
	config    *Config_Port
	server    *Server
	*graceful.Server
}

func newPort(config *Config_Port, s *Server) (p *port, err error) {
	// load rsa private key if pem file name is set
	var pk *rsa.PrivateKey
	if fname := config.GetRsaPrivatePem(); fname != "" {
		f, err := os.Open(fname)
		if err != nil {
			return nil, err
		}

		fs, err := f.Stat()
		if err != nil {
			return nil, err
		}

		data := make([]byte, fs.Size())
		_, err = f.Read(data)
		if err != nil {
			return nil, err
		}

		pblock, _ := pem.Decode(data)
		if pblock == nil {
			err = fmt.Errorf("pem.Decode fail!")
			return nil, err
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

	mux := http.NewServeMux()

	p = &port{
		appSet: make(map[string]*AppBase),
		rsaPK:  pk,
		config: config,
		server: s,
		Server: &graceful.Server{
			Server: &http.Server{
				Addr:    config.GetAddr(),
				Handler: mux,
			},
		},
	}

	mux.HandleFunc("/", p.wsHandler)
	return
}

func (p *port) wsHandler(w http.ResponseWriter, req *http.Request) {
	var err error
	defer p.server.printErr(&err, "port.wsHandler", p.config.GetAddr(), req.RemoteAddr)

	uri, _ := url.QueryUnescape(req.URL.RequestURI())
	subMatch := appNameReg.FindStringSubmatch(uri)
	if len(subMatch) < 2 {
		err = fmt.Errorf("Invalid URI(%q)!", uri)
		return
	}

	appName := subMatch[1]
	app := p.getApp(appName)
	if app == nil {
		err = fmt.Errorf("App not found! appName=%q", appName)
		return
	}

	ws, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}

	newPeerBase(ws, app, p.rsaPK)
}

func (p *port) ListenAndServe() error {
	cert := p.config.GetTlsCertFile()
	key := p.config.GetTlsKeyFile()
	if cert == "" && key == "" {
		return p.Server.ListenAndServe()
	} else {
		return p.Server.ListenAndServeTLS(cert, key)
	}
}

func (p *port) addApp(app *AppBase) {
	p.appSetMut.Lock()
	defer p.appSetMut.Unlock()
	if app != nil {
		p.appSet[app.Name()] = app
	}
}

func (p *port) delApp(appName string) {
	p.appSetMut.Lock()
	defer p.appSetMut.Unlock()
	delete(p.appSet, appName)
}

func (p *port) getApp(appName string) *AppBase {
	p.appSetMut.RLock()
	defer p.appSetMut.RUnlock()
	return p.appSet[appName]
}
