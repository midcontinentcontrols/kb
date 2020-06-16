package logger

import "go.uber.org/zap"

type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
}

type fakeLogger struct{}

func (l *fakeLogger) Info(msg string, fields ...zap.Field)  {}
func (l *fakeLogger) Error(msg string, fields ...zap.Field) {}
func (l *fakeLogger) Debug(msg string, fields ...zap.Field) {}
func (l *fakeLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *fakeLogger) With(fields ...zap.Field) Logger       { return l }

func NewFakeLogger() Logger {
	return &fakeLogger{}
}
