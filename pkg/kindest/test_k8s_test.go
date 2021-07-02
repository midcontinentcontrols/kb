package kindest

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/test"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/stretchr/testify/require"
)

var kindestTestImageName = "kindest-example"

func TestTestK8sEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		pushRepo := test.GetPushRepository()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
		specYaml := fmt.Sprintf(`build:
  name: %s/%s
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
`, pushRepo, kindestTestImageName, pushRepo, kindestTestImageName)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
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
		pushRepo := test.GetPushRepository()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "exit 1"]`
		specYaml := fmt.Sprintf(`build:
  name: %s/%s
test:
  - name: basic
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
`, pushRepo, kindestTestImageName, pushRepo, kindestTestImageName)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
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
		pushRepo := test.GetPushRepository()
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
		specYaml := fmt.Sprintf(`build:
  name: %s/%s
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
`, pushRepo, kindestTestImageName, pushRepo, kindestTestImageName)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{
			Kind:      name,
			Transient: true,
		}, log)
		require.NoError(t, err)
	})
}
