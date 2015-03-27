package nest2go

import (
	"fmt"
	"io"
	"os"
)

type ConsoleLogger chan *LogRecord

func NewConsoleLogger() ConsoleLogger {
	records := make(ConsoleLogger, 64)
	go records.run(os.Stdout)
	return records
}

func (w ConsoleLogger) run(out io.Writer) {
	for rec := range w {
		fmt.Fprint(out, rec.RawMsg())
	}
}

func (w ConsoleLogger) Debug(args ...interface{}) { w.log(LOG_LEVEL_DEBUG, args...) }
func (w ConsoleLogger) Trace(args ...interface{}) { w.log(LOG_LEVEL_TRACE, args...) }
func (w ConsoleLogger) Info(args ...interface{})  { w.log(LOG_LEVEL_INFO, args...) }
func (w ConsoleLogger) Warn(args ...interface{})  { w.log(LOG_LEVEL_WARN, args...) }
func (w ConsoleLogger) Error(args ...interface{}) { w.log(LOG_LEVEL_ERROR, args...) }

func (w ConsoleLogger) Debugf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_DEBUG, format, args...)
}
func (w ConsoleLogger) Tracef(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_TRACE, format, args...)
}
func (w ConsoleLogger) Infof(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_INFO, format, args...)
}
func (w ConsoleLogger) Warnf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_WARN, format, args...)
}
func (w ConsoleLogger) Errorf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_ERROR, format, args...)
}

func (w ConsoleLogger) log(lv LogLevel, args ...interface{}) {
	w.logWrite(lv, fmt.Sprint(args...))
}

func (w ConsoleLogger) logf(lv LogLevel, format string, args ...interface{}) {
	w.logWrite(lv, fmt.Sprintf(format, args...))
}

// This is the ConsoleLogWriter's output method.  This will block if the output
// buffer is full.
func (w ConsoleLogger) logWrite(lv LogLevel, msg string) {
	w <- NewLogRecord(lv, msg, 4)
}

// Close stops the logger from sending messages to standard output.  Attempts to
// send log messages to this logger after a Close have undefined behavior.
func (w ConsoleLogger) Close() {
	close(w)
}
