package log

import (
	"fmt"
	. "github.com/lawwong/nest2go/protomsg"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const logFileExt = ".log"

type filePathInfo struct {
	dir  string
	name string
	sep  Config_LogSepTime
}

type FileLogger struct {
	levelMut sync.RWMutex
	level    LogLevel

	pathInfo       filePathInfo
	filePathPrefix string

	filePath chan filePathInfo
	rec      chan *LogRecord
	quit     chan struct{}
}

func NewFileLogger() *FileLogger {
	fl := &FileLogger{
		filePath: make(chan filePathInfo),
		rec:      make(chan *LogRecord),
		quit:     make(chan struct{}),
	}
	go fl.run()
	return fl
}

func (w *FileLogger) Dir() string {
	return w.pathInfo.dir
}

func (w *FileLogger) Name() string {
	return w.pathInfo.name
}

func (w *FileLogger) Sep() Config_LogSepTime {
	return w.pathInfo.sep
}

func (w *FileLogger) Level() LogLevel {
	w.levelMut.RLock()
	defer w.levelMut.RUnlock()
	return w.level
}

func (w *FileLogger) SetLevel(l LogLevel) {
	w.levelMut.Lock()
	defer w.levelMut.Unlock()
	w.level = l
}

func (w *FileLogger) SetFilePath(dir, name string) {
	info := filePathInfo{
		dir:  dir,
		name: name,
		sep:  w.pathInfo.sep,
	}
	select {
	case w.filePath <- info:
	case <-w.quit:
	}
}

func (w *FileLogger) SetFileSepTime(dur Config_LogSepTime) {
	info := filePathInfo{
		dir:  w.pathInfo.dir,
		name: w.pathInfo.name,
		sep:  dur,
	}
	select {
	case w.filePath <- info:
	case <-w.quit:
	}
}

func (w *FileLogger) needSepFile(last, cur time.Time) bool {
	switch w.pathInfo.sep {
	case Config_YEAR:
		return last.Year() != cur.Year()
	case Config_MOUNTH:
		return last.Month() != cur.Month() || last.Year() != cur.Year()
	case Config_DAY:
		return last.Day() != cur.Day() || cur.Sub(last) > 24*time.Hour
	case Config_HOUR:
		return last.Hour() != cur.Hour() || cur.Sub(last) > time.Hour
	case Config_MINUTE:
		return last.Minute() != cur.Minute() || cur.Sub(last) > time.Minute
	case Config_SECOND:
		return last.Second() != cur.Second() || cur.Sub(last) > time.Second
	}
	return false
}

func (w *FileLogger) filePathWithTime(t time.Time) string {
	p := w.filePathPrefix
	switch w.pathInfo.sep {
	case Config_YEAR:
		p += t.Format("_06")
	case Config_MOUNTH:
		p += t.Format("_0601")
	case Config_DAY:
		p += t.Format("_060102")
	case Config_HOUR:
		p += t.Format("_060102_15")
	case Config_MINUTE:
		p += t.Format("_060102_1504")
	case Config_SECOND:
		p += t.Format("_060102_150405")
	}
	return p + logFileExt
}

func (w *FileLogger) run() {
	var fPathSet bool
	var lastRecTime time.Time
	var f *os.File
	var fPath string
	var err error

	for {
		select {
		case info := <-w.filePath:
			if w.pathInfo != info {
				w.pathInfo = info
				w.filePathPrefix = filepath.Join(info.dir, info.name)
				lastRecTime = time.Time{}
				err = nil
			}
		case rec := <-w.rec:
			if err != nil || !fPathSet {
				fmt.Printf("%s:%s", w.filePathPrefix, rec.RawMsg())
				continue
			}

			if f == nil || w.needSepFile(lastRecTime, rec.Created) {
				if f != nil {
					f.Close()
					f = nil
				}
				fPath = w.filePathWithTime(rec.Created)
				f, err = os.OpenFile(
					fPath,
					os.O_WRONLY|os.O_CREATE|os.O_APPEND,
					0664)
				if err != nil {
					fmt.Printf("open file %q fail! %s\n", fPath, err)
					fmt.Printf("%s:%s", w.filePathPrefix, rec.RawMsg())
					continue
				}
			}

			_, err = f.WriteString(rec.RawMsg())
			if err != nil {
				fmt.Printf("write string to %q fail! %s\n", fPath, err)
				fmt.Printf("%s:%s", w.filePathPrefix, rec.RawMsg())
				f.Close()
				f = nil
			}

			lastRecTime = rec.Created
		case <-w.quit:
			if f != nil {
				f.Close()
				f = nil
			}
			break
		}
	}
}

func (w *FileLogger) Debug(args ...interface{}) {
	w.log(LOG_LEVEL_DEBUG, args...)
}
func (w *FileLogger) Trace(args ...interface{}) {
	w.log(LOG_LEVEL_TRACE, args...)
}
func (w *FileLogger) Info(args ...interface{}) {
	w.log(LOG_LEVEL_INFO, args...)
}
func (w *FileLogger) Warn(args ...interface{}) {
	w.log(LOG_LEVEL_WARN, args...)
}
func (w *FileLogger) Error(args ...interface{}) {
	w.log(LOG_LEVEL_ERROR, args...)
}

func (w *FileLogger) Debugf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_DEBUG, format, args...)
}
func (w *FileLogger) Tracef(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_TRACE, format, args...)
}
func (w *FileLogger) Infof(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_INFO, format, args...)
}
func (w *FileLogger) Warnf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_WARN, format, args...)
}
func (w *FileLogger) Errorf(format string, args ...interface{}) {
	w.logf(LOG_LEVEL_ERROR, format, args...)
}

func (w *FileLogger) log(lv LogLevel, args ...interface{}) {
	w.levelMut.RLock()
	defer w.levelMut.RUnlock()
	if lv >= w.level {
		w.logWrite(lv, fmt.Sprint(args...))
	}
}

func (w *FileLogger) logf(lv LogLevel, format string, args ...interface{}) {
	w.levelMut.RLock()
	defer w.levelMut.RUnlock()
	if lv >= w.level {
		w.logWrite(lv, fmt.Sprintf(format, args...))
	}
}

// This is the ConsoleLogWriter's output method.  This will block if the output
// buffer is full.
func (w *FileLogger) logWrite(lv LogLevel, msg string) {
	w.logWriteRec(NewLogRecord(lv, msg, 4))
}

// This is the ConsoleLogWriter's output method.  This will block if the output
// buffer is full.
func (w *FileLogger) logWriteRec(rec *LogRecord) {
	select {
	case w.rec <- rec:
	case <-w.quit:
	}
}

// Close stops the logger from sending messages to standard output.  Attempts to
// send log messages to this logger after a Close have undefined behavior.
func (w *FileLogger) Close() {
	close(w.quit)
}
