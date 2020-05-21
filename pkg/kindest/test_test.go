package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNoTests(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	defer os.RemoveAll(filepath.Dir(specPath))
	require.Equal(t, ErrNoTests, Test(
		&TestOptions{
			File: specPath,
		},
	))
}

func TestTest(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
test:
  - name: basic
    build:
      name: midcontinentcontrols/kindest-basic-test
      docker:
        dockerfile: Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File: specPath,
		},
	))
}
