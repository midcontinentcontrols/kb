package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newTestLogger() logger.Logger {
	if os.Getenv("DEBUG") == "1" {
		return logger.NewZapLoggerFromEnv()
	}
	return logger.NewFakeLogger()
}

type testEnv struct {
	files map[string]interface{}
}

func createFiles(files map[string]interface{}, dir string) error {
	if err := os.MkdirAll(dir, 0766); err != nil {
		return err
	}
	for k, v := range files {
		path := filepath.Join(dir, k)
		if m, ok := v.(map[string]interface{}); ok {
			if err := createFiles(m, path); err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
		} else if s, ok := v.(string); ok {
			if err := ioutil.WriteFile(
				path,
				[]byte(s),
				0644,
			); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown type for %s", path)
		}
	}
	return nil
}

func runTest(
	t *testing.T,
	files func(name string) map[string]interface{},
	f func(t *testing.T, rootPath string),
) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, createFiles(files(name), rootPath))
	f(t, rootPath)
}
