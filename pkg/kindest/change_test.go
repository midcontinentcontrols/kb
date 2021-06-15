package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/stretchr/testify/require"
)

//
// This test detects which modules need to be rebuilt based on a list
// of files that have changed. All submodules have use the root module's
// directory as their build context, allowing modules to depend on each
// other.
func TestModuleChanged(t *testing.T) {
	name := RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0644))
	defer func() {
		require.NoError(t, os.RemoveAll(rootPath))
	}()
	depYaml := fmt.Sprintf(`build:
  name: %s-dep
  context: ..`, name)
	subYaml := fmt.Sprintf(`build:
  name: %s-sub
  context: ..`, name)
	specYaml := fmt.Sprintf(`dependencies:
- dep
- sub
build:
  name: %s`, name)
	depDockerfile := `FROM alpine:3.11.6
COPY dep dep
CMD ["sh", "-c", "echo \"Hello, world\""]`
	subDockerfile := `FROM alpine:3.11.6
COPY foo/bar bar
CMD ["sh", "-c", "echo \"Hello, world\""]`
	dockerfile := fmt.Sprintf(`FROM %s-dep:latest
CMD ["sh", "-c", "echo \"foo bar baz\""]`, name)
	files := map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"dep": map[string]interface{}{
			"kindest.yaml": depYaml,
			"Dockerfile":   depDockerfile,
		},
		"foo": map[string]interface{}{
			"bar": "baz",
		},
		"sub": map[string]interface{}{
			"kindest.yaml": subYaml,
			"Dockerfile":   subDockerfile,
		},
	}
	require.NoError(t, createFiles(files, rootPath))
	// Modify a file outside of 'dep' and ensure it doesn't cause dep to be rebuilt
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "foo", "bar"),
		[]byte("hello"),
		0644,
	))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	abs, err := filepath.Abs(filepath.Join(rootPath, "foo", "bar"))
	require.NoError(t, err)
	changed, err := module.GetAffectedModules([]string{abs})
	require.NoError(t, err)
	require.Equal(t, 1, len(changed))
	abs, err = filepath.Abs(filepath.Join(rootPath, "sub", "kindest.yaml"))
	require.NoError(t, err)
	require.Equal(t, abs, changed[0].Path)
}
