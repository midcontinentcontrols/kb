package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/util"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/stretchr/testify/require"
)

func TestTestErrNoEnv(t *testing.T) {
	name := util.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	_, err := p.GetModule(specPath)
	require.Equal(t, ErrNoTestEnv, err)
}

func TestTestErrMultipleEnv(t *testing.T) {
	name := util.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
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
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	_, err := p.GetModule(specPath)
	require.Equal(t, ErrMultipleTestEnv, err)
}
