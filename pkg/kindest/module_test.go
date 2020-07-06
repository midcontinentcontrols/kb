package kindest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestModule(t *testing.T) *Module {
	return nil
}

func TestModule(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	require.NoError(t, os.MkdirAll(name, 0644))
	defer func() {
		require.NoError(t, os.Remove(name))
	}()
	//	specYaml := fmt.Sprintf(`build:
	//  name: %s`, name)
	//	dockerfile := `FROM alpine:3.11.6
	//CMD ["sh", "-c", "echo \"Hello, world\""]`
	//createFiles(t, name, map[string]interface{}{
	//	"kindest.yaml": specYaml,
	//	"Dockerfile":   dockerfile,
	//})
	spec := &KindestSpec{}
	module := NewModule(
		spec,
		filepath.Join(name, "kindest.yaml"),
		nil,
		logger.NewFakeLogger(),
	)
	require.NoError(t, module.Build())
}
