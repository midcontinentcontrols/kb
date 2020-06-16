package main

import (
	"fmt"
	"os"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Error("main", zap.String("err", err.Error()))
		os.Exit(1)
	}
}
