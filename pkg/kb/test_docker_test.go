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

func TestTestDockerEnv(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		dockerfile := `FROM alpine:3.11.6
RUN apk add --no-cache bash
CMD ["bash", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
		specYaml := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      docker: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{BuildOptions: BuildOptions{NoPush: true}}, log)
		require.NoError(t, err)
	})
}

func TestTestDockerError(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "exit 1"]`
		specYaml := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      docker: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{BuildOptions: BuildOptions{NoPush: true}}, log)
		require.Error(t, err)
		require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
	})
}

func TestTestDockerMount(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		script := `#!/bin/bash
set -euo pipefail
if [ "$(cat /data/foo)" -ne "bar" ]; then
  exit 120
fi
exit 121`
		dockerfile := `FROM alpine:3.11.6
RUN apk add --no-cache bash
COPY script.sh /usr/bin/entry
RUN chmod +x /usr/bin/entry
CMD ["entry"]`
		specYaml := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    variables:
      - name: EXPECTED_VALUE
        value: bar
    env:
      docker:
        volumes:
          - source: data-dir
            target: /data
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kb.yaml":    specYaml,
			"Dockerfile": dockerfile,
			"script.sh":  script,
			"data-dir": map[string]interface{}{
				"foo": "bar",
			},
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		p := NewProcess(runtime.NumCPU(), log)
		module, err := p.GetModule(filepath.Join(rootPath, "kb.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		err = module.RunTests(&TestOptions{BuildOptions: BuildOptions{NoPush: true}}, log)
		require.Error(t, err)
		// The command should fail with an exotic exit code
		// to indicate the files were mounted
		require.Contains(t, err.Error(), "exit code 121")
	})
}
