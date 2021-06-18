package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
)

func ExecCommand(log logger.Logger, command string, args ...interface{}) error {
	var interpolated string
	if len(args) > 0 {
		interpolated = fmt.Sprintf(command, args...)
	} else {
		interpolated = command
	}
	cmd := exec.Command("bash", "-c", interpolated)
	log.Info("Running command", zap.String("cmd", interpolated))
	r, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	resultStdout := make(chan interface{}, 1)
	resultStderr := make(chan interface{}, 1)
	go func() {
		defer close(resultStderr)
		stderr, err := ioutil.ReadAll(r)
		if err != nil && err != io.EOF && err != os.ErrClosed && !strings.Contains(err.Error(), "file already closed") {
			resultStderr <- fmt.Errorf("ReadAll: %v", err)
			return
		}
		resultStderr <- string(stderr)
	}()
	go func() {
		defer close(resultStdout)
		stdout, err := ioutil.ReadAll(stdout)
		if err != nil && err != io.EOF && err != os.ErrClosed && !strings.Contains(err.Error(), "file already closed") {
			resultStdout <- fmt.Errorf("ReadAll: %v", err)
			return
		}
		resultStdout <- string(stdout)
	}()
	//defer NewWaitingMessage(interpolated, 5*time.Second, log).Stop()
	if err := cmd.Run(); err != nil && err != io.EOF && err != os.ErrClosed {
		stdoutStr, _ := (<-resultStdout).(string)
		vStderr := <-resultStderr
		if stderr, ok := vStderr.(string); ok {
			return fmt.Errorf("%v: stdout: %s\nstderr: %s", err, stdoutStr, stderr)
		} else if err2, ok := vStderr.(error); ok {
			return fmt.Errorf("%v: stdout: %s\nstderr: %s", err, err2, stdoutStr)
		} else {
			panic("unreachable branch detected")
		}
	}
	return nil
}
