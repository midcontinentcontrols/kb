package kindest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestModule(t *testing.T) *Module {
	return nil
}

func TestModuleBuildStatus(t *testing.T) {
	t.Run("BuildStatusSucceeded", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
	t.Run("BuildStatusFailed", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
RUN exit 1`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		err = module.Build(&BuildOptions{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "docker: The command '/bin/sh -c exit 1' returned a non-zero code: 1")
		require.Equal(t, BuildStatusFailed, module.Status())
	})
}
