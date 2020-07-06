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
	with  [][]zap.Field
}

func NewMockLogger(base Logger) *MockLogger {
	log := new(MockLogger)
	log.base = base
	return log
}

var ErrNotObserved = fmt.Errorf("log entry was not observed")

func (l *MockLogger) combinedFields(fields []zap.Field) []zap.Field {
	for _, with := range l.with {
		for _, item := range with {
			fields = append(fields, item)
		}
	}
	return fields
}

func (l *MockLogger) WasObserved(level string, msg string, fields ...zap.Field) bool {
	l.l.Lock()
	defer l.l.Unlock()
	numFields := len(fields)
	for _, line := range l.lines {
		if line.level == level && line.message == msg {
			if numFields != len(line.fields) {
				continue
			}
			allMatch := true
			for _, field := range fields {
				for _, other := range line.fields {
					if !field.Equals(other) {
						allMatch = false
						break
					}
				}
				if !allMatch {
					break
				}
			}
			if allMatch {
				return true
			}
		}
	}
	return false
}

func (l *MockLogger) WasObservedIgnoreFields(level string, msg string) bool {
	l.l.Lock()
	defer l.l.Unlock()
	for _, line := range l.lines {
		if line.level == level && line.message == msg {
			return true
		}
	}
	return false
}

func (l *MockLogger) Info(msg string, fields ...zap.Field) {
	l.l.Lock()
	defer l.l.Unlock()
	fields = l.combinedFields(fields)
	l.base.Info(msg, fields...)
	l.lines = append(l.lines, &logLine{
		level:   "info",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Error(msg string, fields ...zap.Field) {
	l.l.Lock()
	defer l.l.Unlock()
	fields = l.combinedFields(fields)
	l.base.Error(msg, fields...)
	l.lines = append(l.lines, &logLine{
		level:   "error",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Debug(msg string, fields ...zap.Field) {
	l.l.Lock()
	defer l.l.Unlock()
	fields = l.combinedFields(fields)
	l.base.Debug(msg, fields...)
	l.lines = append(l.lines, &logLine{
		level:   "debug",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) Warn(msg string, fields ...zap.Field) {
	l.l.Lock()
	defer l.l.Unlock()
	fields = l.combinedFields(fields)
	l.base.Warn(msg, fields...)
	l.lines = append(l.lines, &logLine{
		level:   "warn",
		message: msg,
		fields:  fields,
	})
}

func (l *MockLogger) With(fields ...zap.Field) Logger {
	l.l.Lock()
	defer l.l.Unlock()
	l.with = append(l.with, fields)
	return l
}
