package nest2go

import (
	"fmt"
	"github.com/lawwong/nest2go/log"
	"strings"
)

type Signal struct{}

var SIGNAL = Signal{}

// Errors introduced by the nest2go server.
var (
	ErrInvalidArgument       = fmt.Errorf("invalid argument")
	ErrMultipleRegistrations = fmt.Errorf("multiple registrations")
	ErrServiceClosed         = fmt.Errorf("service closed")
	ErrHandlerNotFound       = fmt.Errorf("app handler not found")
	ErrPortNotFound          = fmt.Errorf("port not found")
	ErrAppNotFound           = fmt.Errorf("app not found")
)

func printErr(log *log.FileLogger, err *error, funcName string, args ...interface{}) {
	if log == nil {
		return
	}
	argsStrs := make([]string, len(args))
	for i, arg := range args {
		argsStrs[i] = fmt.Sprintf("%v", arg)
	}
	argsStr := strings.Join(argsStrs, ",")
	if err != nil && *err != nil {
		log.Warnf("%s(%s) fail! %v", funcName, argsStr, *err)
	} else {
		log.Infof("%s(%s) ok!", funcName, argsStr)
	}
}
