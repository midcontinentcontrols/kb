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

func TestTestDockerEnv(t *testing.T) {
	name := util.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{NoPush: true}, p, log)
	require.NoError(t, err)
}

func TestTestDockerError(t *testing.T) {
	name := util.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{NoPush: true}, p, log)
	require.Error(t, err)
	require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
}

func TestTestDockerMount(t *testing.T) {
	name := util.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
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
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"script.sh":    script,
		"data-dir": map[string]interface{}{
			"foo": "bar",
		},
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.RunTests(&TestOptions{NoPush: true}, p, log)
	require.Error(t, err)
	// The command should fail with an exotic exit code
	// to indicate the files were mounted
	require.Contains(t, err.Error(), "exit code 121")
}
