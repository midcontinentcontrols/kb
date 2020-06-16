package logger

import "go.uber.org/zap"

type fakeLogger struct{}

func (l *fakeLogger) Info(msg string, fields ...zap.Field)  {}
func (l *fakeLogger) Error(msg string, fields ...zap.Field) {}
func (l *fakeLogger) Debug(msg string, fields ...zap.Field) {}
func (l *fakeLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *fakeLogger) With(fields ...zap.Field) Logger       { return l }

func NewFakeLogger() Logger {
	return &fakeLogger{}
}
