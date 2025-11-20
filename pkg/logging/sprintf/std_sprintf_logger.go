package logging

import (
	"fmt"
	"log"
	"strings"
)

func NewStdSprintfLogger() SprintfLogger {
	return newStdSprintfLogger([]string{})
}

func newStdSprintfLogger(parts []string) SprintfLogger {
	prefix := strings.Join(parts, ", ")
	return &sprintfLogger{parts: parts, prefix: prefix}
}

type sprintfLogger struct {
	parts  []string
	prefix string
}

func (l *sprintfLogger) Level() string {
	return "debug"
}

func (l *sprintfLogger) logf(msg string, args ...interface{}) {
	str := fmt.Sprintf(msg, args...)
	if l.prefix != "" {
		str = fmt.Sprintf("%s , %s", l.prefix, str)
	}
	if !strings.HasSuffix(str, "\n") {
		log.Println(str)
	} else {
		log.Print(str)
	}
}

func (l *sprintfLogger) Debugf(msg string, args ...interface{}) {
	l.logf(msg, args...)
}

func (l *sprintfLogger) Infof(msg string, args ...interface{}) {
	l.logf(msg, args...)
}

func (l *sprintfLogger) Warnf(msg string, args ...interface{}) {
	l.logf(msg, args...)
}

func (l *sprintfLogger) Errorf(msg string, args ...interface{}) {
	l.logf(msg, args...)
}

func (l *sprintfLogger) With(args ...interface{}) SprintfLogger {
	parts := make([]string, len(l.parts)+len(args)/2)
	copy(parts, l.parts)
	for i := 0; i < len(args); i = i + 2 {
		argK := args[i]
		argV := interface{}("(nil)")
		if i+1 < len(args) {
			argV = args[i+1]
		}
		parts = append(parts, fmt.Sprintf("%v: %v", argK, argV))
	}
	return newStdSprintfLogger(parts)
}
