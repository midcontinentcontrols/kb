package kb

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kb/pkg/test"

	"github.com/midcontinentcontrols/kb/pkg/logger"

	"github.com/stretchr/testify/require"
)

func TestTestK8sEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		pushImage := test.GetPushImage()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
		specYaml := fmt.Sprintf(`build:
  name: %s
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      kubernetes: {}
    build:
      name: %s-test
      dockerfile: Dockerfile
`, pushImage, pushImage)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{
			Kind:      name,
			Transient: true,
		}, log)
		require.NoError(t, err)
	})
}

func TestTestK8sError(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		pushImage := test.GetPushImage()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "exit 1"]`
		specYaml := fmt.Sprintf(`build:
  name: %s
test:
  - name: basic
    env:
      kubernetes: {}
    build:
      name: %s-test
      dockerfile: Dockerfile
`, pushImage, pushImage)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{
			Kind:      name,
			Transient: true,
		}, log)
		require.Error(t, err)
		require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
	})
}

func TestTestK8sKindEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		pushImage := test.GetPushImage()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
		specYaml := fmt.Sprintf(`build:
  name: %s
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      kubernetes: {}
    build:
      name: %s-test
      dockerfile: Dockerfile
`, pushImage, pushImage)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{
			Kind:      name,
			Transient: true,
		}, log)
		require.NoError(t, err)
	})
}
