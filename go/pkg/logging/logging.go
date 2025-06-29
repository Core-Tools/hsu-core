package logging

const (
	LogLevelDebug = 0
	LogLevelInfo  = 1
	LogLevelWarn  = 2
	LogLevelError = 3
)

type Logger interface {
	LogLevelf(level int, format string, args ...interface{})
	Debugf(msg string, args ...interface{})
	Infof(msg string, args ...interface{})
	Warnf(msg string, args ...interface{})
	Errorf(msg string, args ...interface{})
}

type LogLevelFunc func(level int, format string, args ...interface{})
type LogFunc func(format string, args ...interface{})

type LogFuncs struct {
	LogLevelf LogLevelFunc
	Debugf    LogFunc
	Infof     LogFunc
	Warnf     LogFunc
	Errorf    LogFunc
}

type logger struct {
	prefix string
	funcs  LogFuncs
}

func NewLogger(prefix string, funcs LogFuncs) Logger {
	return &logger{
		prefix: prefix,
		funcs:  funcs,
	}
}

func (l *logger) logf(level int, msg string, args ...interface{}) {
	if l.prefix != "" {
		msg = l.prefix + msg
	}
	if l.funcs.LogLevelf != nil {
		l.funcs.LogLevelf(level, msg, args...)
		return
	}
	switch level {
	case LogLevelDebug:
		if l.funcs.Debugf != nil {
			l.funcs.Debugf(msg, args...)
		}
	case LogLevelInfo:
		if l.funcs.Infof != nil {
			l.funcs.Infof(msg, args...)
		}
	case LogLevelWarn:
		if l.funcs.Warnf != nil {
			l.funcs.Warnf(msg, args...)
		}
	case LogLevelError:
		if l.funcs.Errorf != nil {
			l.funcs.Errorf(msg, args...)
		}
	}
}

func (l *logger) LogLevelf(level int, format string, args ...interface{}) {
	l.logf(level, format, args...)
}

func (l *logger) Debugf(msg string, args ...interface{}) {
	l.logf(LogLevelDebug, msg, args...)
}

func (l *logger) Infof(msg string, args ...interface{}) {
	l.logf(LogLevelInfo, msg, args...)
}

func (l *logger) Warnf(msg string, args ...interface{}) {
	l.logf(LogLevelWarn, msg, args...)
}

func (l *logger) Errorf(msg string, args ...interface{}) {
	l.logf(LogLevelError, msg, args...)
}
