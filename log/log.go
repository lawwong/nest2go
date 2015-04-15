package log

import (
	"fmt"
	"runtime"
	"time"
)

type LogLevel int

const (
	LOG_LEVEL_DEBUG LogLevel = iota
	LOG_LEVEL_TRACE
	LOG_LEVEL_INFO
	LOG_LEVEL_WARN
	LOG_LEVEL_ERROR
	LOG_LEVEL_NONE
)

// Logging level strings
var (
	levelStrings = [...]string{"DEBG", "TRAC", "INFO", "WARN", "EROR"}
)

type Logger interface {
	Debug(args ...interface{})
	Trace(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})

	Debugf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	Level() LogLevel
	SetLev(l LogLevel)
}

type LogRecord struct {
	Level   LogLevel  // The log level
	Created time.Time // The time at which the log message was created (nanoseconds)
	Source  string    // The message source
	Message string    // The log message
}

func NewLogRecord(lv LogLevel, msg string, callDegree int) *LogRecord {
	// Determine caller func
	src := ""
	if lv <= LOG_LEVEL_TRACE {
		pc, _, lineno, ok := runtime.Caller(callDegree)
		if ok {
			src = fmt.Sprintf("%s:%d", runtime.FuncForPC(pc).Name(), lineno)
		}
	}

	return &LogRecord{
		Level:   lv,
		Created: time.Now(),
		Source:  src,
		Message: msg,
	}
}

func (r *LogRecord) RawMsg() string {
	return fmt.Sprintf("%s [%s]%s %s\n", r.Created.Format("06/01/02 15:04:05.000"), levelStrings[r.Level], r.Source, r.Message)
}
