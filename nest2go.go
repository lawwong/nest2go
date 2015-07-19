package nest2go

import (
	"errors"
	"fmt"
	"github.com/lawwong/nest2go/log"
	"strings"
)

// Errors introduced by the nest2go server.
var (
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrMultipleRegistrations = errors.New("multiple registrations")
	ErrServiceClosed         = errors.New("service closed")
	ErrHandlerNotFound       = errors.New("app handler not found")
	ErrPortNotFound          = errors.New("port not found")
	ErrAppNotFound           = errors.New("app not found")
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
