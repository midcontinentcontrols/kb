package kb

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kb/pkg/logger"
	"github.com/midcontinentcontrols/kb/pkg/test"
	"github.com/stretchr/testify/require"
)

// This test detects which modules need to be rebuilt based on a list
// of files that have changed. All submodules have use the root module's
// directory as their build context, allowing modules to depend on each
// other.
func TestModuleChanged(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
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
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
			"dep": map[string]interface{}{
				"kb.yaml":    depYaml,
				"Dockerfile": depDockerfile,
			},
			"foo": map[string]interface{}{
				"bar": "baz",
			},
			"sub": map[string]interface{}{
				"kb.yaml":    subYaml,
				"Dockerfile": subDockerfile,
			},
		}))
		// Modify a file outside of 'dep' and ensure it doesn't cause dep to be rebuilt
		abs, err := filepath.Abs(filepath.Join(rootPath, "foo", "bar"))
		require.NoError(t, err)
		require.NoError(t, ioutil.WriteFile(
			abs,
			[]byte("hello"),
			0644,
		))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		changed, err := module.GetAffectedModules([]string{abs})
		require.NoError(t, err)
		require.Equal(t, 1, len(changed))
		subPath, err := filepath.Abs(filepath.Join(rootPath, "sub", "kb.yaml"))
		require.NoError(t, err)
		require.Equal(t, subPath, changed[0].Path)
	})
}
