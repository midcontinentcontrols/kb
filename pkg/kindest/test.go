package kindest

import (
	"fmt"

	"go.uber.org/zap"
)

type TestOptions struct {
	File string `json:"file,omitempty"`
}

var ErrNoTests = fmt.Errorf("no tests configured")

func Test(options *TestOptions) error {
	spec, manifestPath, err := loadSpec(options.File)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	if len(spec.Test) == 0 {
		return ErrNoTests
	}
	return nil
}
