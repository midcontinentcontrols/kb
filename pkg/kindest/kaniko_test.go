package kindest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildKanikoLocal(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	defer os.RemoveAll(filepath.Dir(specPath))
	require.NoError(t, Build(&BuildOptions{
		File:    specPath,
		Builder: "docker",
	}))
}
