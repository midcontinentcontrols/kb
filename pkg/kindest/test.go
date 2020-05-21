package kindest

import "go.uber.org/zap"

type TestOptions struct {
	File string `json:"file,omitempty"`
}

func Test(options *TestOptions) error {
	_, manifestPath, err := loadSpec(options.File)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	return nil
}
