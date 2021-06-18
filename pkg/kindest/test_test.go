package kindest

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/test"

	"github.com/stretchr/testify/require"
)

func TestTestErrNoEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
		specYaml := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		_, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.Equal(t, ErrNoTestEnv, err)
	})
}

func TestTestErrMultipleEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
		specYaml := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes: {}
      docker: {}
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		_, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.Equal(t, ErrMultipleTestEnv, err)
	})
}
