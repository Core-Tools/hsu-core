package logging

type SprintfLogger interface {
	Level() string
	Debugf(msg string, args ...interface{})
	Infof(msg string, args ...interface{})
	Warnf(msg string, args ...interface{})
	Errorf(msg string, args ...interface{})
	With(args ...interface{}) SprintfLogger
}
