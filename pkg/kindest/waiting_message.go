package kindest

import (
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
)

type WaitingMessage struct {
	exit chan<- int
}

func NewWaitingMessage(name string, delay time.Duration, log logger.Logger) *WaitingMessage {
	exit := make(chan int, 1)
	go func() {
		iterations := 0
		for {
			select {
			case <-exit:
				return
			case <-time.After(delay):
				iterations++
				elapsed := delay * time.Duration(iterations)
				log.Info("Waiting", zap.String("name", name), zap.String("elapsed", elapsed.String()))
			}
		}
	}()
	return &WaitingMessage{
		exit: exit,
	}
}

func (w *WaitingMessage) Stop() {
	defer close(w.exit)
	w.exit <- 0
}
