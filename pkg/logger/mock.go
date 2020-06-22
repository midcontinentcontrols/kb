package logger

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

type logLine struct {
	level   string
	message string
	fields  []zap.Field
}

type MockLogger struct {
	base  Logger
	l     sync.Mutex
	lines []*logLine
}

func NewMockLogger(base Logger) *MockLogger {
	log := new(MockLogger)
	log.base = base
	return log
}

var ErrNotObserved = fmt.Errorf("log entry was not observed")

func (l *MockLogger) WasObserved(level string, msg string) error {
	for _, line := range l.lines {
		if line.level == level && line.message == msg {
			return nil
		}
	}
	return ErrNotObserved
}

func (l *MockLogger) Info(msg string, fields ...zap.Field) {
	l.base.Info(msg, fields...)
	l.l.Lock()
	defer l.l.Unlock()
	l.lines = append(l.lines, &logLine{
		level:   "info",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Error(msg string, fields ...zap.Field) {
	l.base.Error(msg, fields...)
	l.l.Lock()
	defer l.l.Unlock()
	l.lines = append(l.lines, &logLine{
		level:   "error",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Debug(msg string, fields ...zap.Field) {
	l.base.Debug(msg, fields...)
	l.l.Lock()
	defer l.l.Unlock()
	l.lines = append(l.lines, &logLine{
		level:   "debug",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Warn(msg string, fields ...zap.Field) {
	l.base.Warn(msg, fields...)
	l.l.Lock()
	defer l.l.Unlock()
	l.lines = append(l.lines, &logLine{
		level:   "warn",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) With(fields ...zap.Field) Logger {
	return l
}
