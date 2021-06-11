package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/stretchr/testify/require"
)

func getPushRepository() string {
	repo, ok := os.LookupEnv("PUSH_REPOSITORY")
	if !ok {
		return "ahemphill"
	}
	return repo
}

func TestTestK8sEnv(t *testing.T) {
	pushRepo := getPushRepository()
	name := "kindest-example"
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
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
`, pushRepo, name, pushRepo, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(specPath)
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	test := p.GetModuleFromTestSpec(specPath, module.Spec.Test[0])
	require.NoError(t, test.Build(&BuildOptions{}))
	err = module.RunTests(&TestOptions{}, log)
	require.NoError(t, err)
}

func TestTestK8sError(t *testing.T) {
	pushRepo := getPushRepository()
	name := "kindest-example"
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "exit 1"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: %s/%s
test:
  - name: basic
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
`, pushRepo, name, pushRepo, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(specPath)
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	test := p.GetModuleFromTestSpec(specPath, module.Spec.Test[0])
	require.NoError(t, test.Build(&BuildOptions{}))
	err = module.RunTests(&TestOptions{}, log)
	require.Error(t, err)
	require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
}
