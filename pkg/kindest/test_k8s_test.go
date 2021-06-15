package kindest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/util"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/stretchr/testify/require"
)

var kindestTestImageName = "kindest-example"

func getPushRepository() string {
	repo, ok := os.LookupEnv("PUSH_REPOSITORY")
	if !ok {
		return "ahemphill"
	}
	return repo
}

func TestTestK8sEnv(t *testing.T) {
	name := util.RandomTestName()
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{
		Kind:      name,
		Transient: true,
	}, p, log)
	require.NoError(t, err)
}

func TestTestK8sError(t *testing.T) {
	name := util.RandomTestName()
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{
		Kind:      name,
		Transient: true,
	}, p, log)
	require.Error(t, err)
	require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
}

func TestTestK8sKindEnv(t *testing.T) {
	name := util.RandomTestName()
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{
		Kind:      name,
		Transient: true,
	}, p, log)
	require.NoError(t, err)
}
