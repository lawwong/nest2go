package nest2go

import (
	"github.com/golang/protobuf/proto"
	. "github.com/lawwong/nest2go/protomsg"
	"os"
)

var DefaultServer *Server

func init() {
	DefaultServer = NewServer()
}

func HandleSetuper(typeName string, setuper Setuper) {
	DefaultServer.HandleSetuper(typeName, setuper)
}

func HandleSetuperFunc(typeName string, setuperFunc SetuperFunc) {
	DefaultServer.HandleSetuperFunc(typeName, setuperFunc)
}

func LoadServerConfigFile(filePath string) error {
	return DefaultServer.LoadConfigFile(filePath)
}

func SaveServerConfigFile(filePath string) error {
	return DefaultServer.SaveConfigFile(filePath)
}

func GetServerConfigFromFile(filePath string, out *Config) (err error) {
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

	err = proto.UnmarshalText(string(data), out)
	return
}

func ServerConfig() *Config {
	return DefaultServer.Config()
}

func UpdateServerConfig() (*Config, error) {
	return DefaultServer.UpdateConfig()
}

func SetServerConfig(conf *Config) error {
	return DefaultServer.SetConfig(conf)
}

func Serve() error {
	return DefaultServer.Serve()
}
