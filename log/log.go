package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

var sourceDir string

func init() {
	_, file, _, _ := runtime.Caller(0)
	sourceDir = regexp.MustCompile(`log\.go`).ReplaceAllString(file, "")
}

// FileWithLineNum File line num
func FileWithLineNum() string {
	for i := 2; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i)

		if ok && (!strings.HasPrefix(file, sourceDir) || strings.HasSuffix(file, "_test.go")) {
			return file + ":" + strconv.FormatInt(int64(line), 10)
		}
	}
	return ""
}

// Colors
const (
	Reset       = "\033[0m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	BlueBold    = "\033[34;1m"
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m"
	YellowBold  = "\033[33;1m"
)

// LogLevel define
type LogLevel int

const (
	LevelSilent LogLevel = iota + 1
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

// Writer log writer interface
type Writer interface {
	Printf(string, ...interface{})
}

// Config log config
type Config struct {
	SlowThreshold time.Duration
	Colorful      bool
	LogLevel      LogLevel
}

// Interface logger interface
type Interface interface {
	LogMode(LogLevel) Interface
	Debug(string, ...interface{})
	D(string, ...interface{})
	Dump(args ...interface{})
	Printf(string, ...interface{})
	Info(string, ...interface{})
	Warn(string, ...interface{})
	Error(string, ...interface{})
	Trace(time.Time, func() (string, int64), error)
}

var (
	// Default log
	Default = New(log.New(os.Stdout, "\r\n", log.LstdFlags), Config{
		SlowThreshold: 200 * time.Millisecond,
		LogLevel:      LevelWarn,
		Colorful:      true,
	})
)

// NewLogger New Logger engine
func NewLogger(out io.Writer, level LogLevel) Interface {
	return New(log.New(out, " ", log.LstdFlags), Config{
		SlowThreshold: 200 * time.Millisecond,
		LogLevel:      level,
		Colorful:      true,
	})
}

// New new log interface
func New(writer Writer, config Config) Interface {
	var (
		debugStr     = "%s\n[debug] "
		infoStr      = "%s\n[info] "
		logStr       = "%s\n[debug] "
		warnStr      = "%s\n[warn] "
		errStr       = "%s\n[error] "
		traceStr     = "%s\n[%.3fms] [rows:%v] %s"
		traceWarnStr = "%s %s\n[%.3fms] [rows:%v] %s"
		traceErrStr  = "%s %s\n[%.3fms] [rows:%v] %s"
	)

	if config.Colorful {
		debugStr = Cyan + "%s\n" + Reset + Green + "[debug] " + Reset
		infoStr = Green + Reset + Green + "[info] " + Reset
		logStr = Green + Reset + Green + "[debug] " + Reset
		warnStr = BlueBold + "%s\n" + Reset + Magenta + "[warn] " + Reset
		errStr = Magenta + "%s\n" + Reset + Red + "[error] " + Reset
		traceStr = Green + "%s\n" + Reset + Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
		traceWarnStr = Green + "%s " + Yellow + "%s\n" + Reset + RedBold + "[%.3fms] " + Yellow + "[rows:%v]" + Magenta + " %s" + Reset
		traceErrStr = RedBold + "%s " + MagentaBold + "%s\n" + Reset + Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
	}

	return &logger{
		Writer:       writer,
		Config:       config,
		debugStr:     debugStr,
		infoStr:      infoStr,
		logStr:       logStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceStr:     traceStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

type logger struct {
	Writer
	Config
	debugStr, infoStr, logStr, warnStr, errStr string
	traceStr, traceErrStr, traceWarnStr        string
}

// LogMode log mode
func (l *logger) LogMode(level LogLevel) Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}

// Info print info
func (l logger) Debug(msg string, data ...interface{}) {
	if l.LogLevel >= LevelDebug {
		l.Printf(l.debugStr+msg, append([]interface{}{FileWithLineNum()}, data...)...)
	}
}

// Info print info
func (l logger) Info(msg string, data ...interface{}) {
	if l.LogLevel >= LevelWarn {
		l.Printf(l.infoStr+msg, data...)
	}
}

// Log print Log
func (l logger) D(msg string, data ...interface{}) {
	if l.LogLevel >= LevelDebug {
		l.Printf(l.logStr+msg, data...)
	}
}

func (l logger) Dump(dat ...interface{}) {
	spew.Dump(dat...)
}

// Warn print warn messages
func (l logger) Warn(msg string, data ...interface{}) {
	if l.LogLevel >= LevelWarn {
		l.Printf(l.warnStr+msg, append([]interface{}{FileWithLineNum()}, data...)...)
	}
}

// Error print error messages
func (l logger) Error(msg string, data ...interface{}) {
	if l.LogLevel >= LevelError {
		l.Printf(l.errStr+msg, append([]interface{}{FileWithLineNum()}, data...)...)
	}
}

// Trace print sql message
func (l logger) Trace(begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel > LevelSilent {
		elapsed := time.Since(begin)
		switch {
		case err != nil && l.LogLevel >= LevelError:
			sql, rows := fc()
			if rows == -1 {
				l.Printf(l.traceErrStr, FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				l.Printf(l.traceErrStr, FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= LevelWarn:
			sql, rows := fc()
			slowLog := fmt.Sprintf("SLOW  >= %v", l.SlowThreshold)
			if rows == -1 {
				l.Printf(l.traceWarnStr, FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				l.Printf(l.traceWarnStr, FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		case l.LogLevel == LevelInfo:
			sql, rows := fc()
			if rows == -1 {
				l.Printf(l.traceStr, FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, "-", sql)
			} else {
				l.Printf(l.traceStr, FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, rows, sql)
			}
		}
	}
}
