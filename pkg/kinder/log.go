package kinder

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var atom zap.AtomicLevel = zap.NewAtomicLevel()
var log *zap.Logger

func setLogLevel() {
	if debug, ok := os.LookupEnv("DEBUG"); ok && debug != "0" {
		atom.SetLevel(zap.DebugLevel)
		return
	}
	logLevel, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		return
	}
	switch logLevel {
	case "info":
		atom.SetLevel(zap.InfoLevel)
	case "debug":
		atom.SetLevel(zap.DebugLevel)
	case "warn":
		atom.SetLevel(zap.WarnLevel)
	case "error":
		atom.SetLevel(zap.ErrorLevel)
	case "dpanic":
		atom.SetLevel(zap.DPanicLevel)
	case "panic":
		atom.SetLevel(zap.PanicLevel)
	case "fatal":
		atom.SetLevel(zap.FatalLevel)
	default:
		panic(fmt.Sprintf("unknown log level '%s'", logLevel))
	}
}

func init() {
	setLogLevel()
	encoderCfg := zap.NewProductionEncoderConfig()
	log = zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atom,
	))
}
