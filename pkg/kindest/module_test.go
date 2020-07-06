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

func TestModule(t *testing.T) {
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
	require.NoError(t, module.Build())
}
