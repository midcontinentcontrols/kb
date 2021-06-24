package kindest

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/distribution/reference"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type BuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Command struct {
	Name string   `json:"name" yaml:"name"`
	Args []string `json:"args" yaml:"args"`
}

type BuildSpec struct {
	Name         string            `json:"name" yaml:"name"`
	Dockerfile   string            `json:"dockerfile,omitempty" yaml:"dockerfile,omitempty"`
	Context      string            `json:"context,omitempty" yaml:"context,omitempty"`
	BuildArgs    []*BuildArg       `json:"buildArgs,omitempty" yaml:"buildArgs,omitempty"`
	Target       string            `json:"target,omitempty" yaml:"target,omitempty"`
	Command      []string          `json:"command,omitempty" yaml:"command,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty"`
	Before       []Command         `json:"before,omitempty" yaml:"before,omitempty"`
	After        []Command         `json:"after,omitempty" yaml:"after,omitempty"`
}

// DependsOnFiles all inputs are absolute paths
func (b *BuildSpec) DependsOnFiles(files []string, manifestPath string) (bool, error) {
	dir := filepath.Dir(manifestPath)

	contextPath := dir
	if b.Context != "" {
		contextPath = filepath.Clean(filepath.Join(contextPath, b.Context))
	}

	dockerfilePath := b.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	dockerfilePath = filepath.Clean(filepath.Join(dir, dockerfilePath))

	include, _, err := createDockerInclude(contextPath, dockerfilePath)
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if file == dockerfilePath {
			return true, nil
		}

		rel, err := filepath.Rel(contextPath, file)
		if err != nil {
			return false, err
		}
		if strings.HasPrefix(rel, "..") {
			// File is outside of build context
			continue
		}
		if include.MatchesPath(rel) {
			return true, nil
		}
	}

	return false, nil
}

func (b *BuildSpec) verifyDocker(manifestPath string, log logger.Logger) error {
	var path string
	if b.Dockerfile != "" {
		path = filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(b.Dockerfile))
	} else {
		path = filepath.Join(filepath.Dir(manifestPath), "Dockerfile")
	}
	path = filepath.Clean(path)
	log.Debug("Resolving Dockerfile", zap.String("name", b.Name), zap.String("path", path))
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("missing Dockerfile at '%s'", path)
	}
	return nil
}

var ErrMissingImageName = fmt.Errorf("missing image name")

func (b *BuildSpec) Verify(manifestPath string, log logger.Logger) error {
	if b.Name == "" {
		return ErrMissingImageName
	}
	return b.verifyDocker(manifestPath, log)
}

func digestPathForManifest(manifestPath string) (string, error) {
	h := md5.New()
	h.Write([]byte(manifestPath))
	name := hex.EncodeToString(h.Sum(nil))
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	path := filepath.Join(u.HomeDir, ".kindest", "digest", name)
	return path, nil
}

type BuildOptions struct {
	NoCache    bool   `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash     bool   `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Builder    string `json:"builder,omitempty" yaml:"builder,omitempty"`
	NoPush     bool   `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
	//Context string `json:"context,omitempty" yaml:"context,omitempty"`
	Force     bool `json:"force,omitempty"`     // If true, will always run docker build regardless of kindest digest
	SkipHooks bool `json:"skipHooks,omitempty"` // If true, skip before/after build hooks
	Verbose   bool `json:"verbose,omitempty" yaml:"verbose,omitempty"`
}

func locateSpec(file string) (string, error) {
	if file != "" {
		var err error
		file, err = filepath.Abs(file)
		if err != nil {
			return "", err
		}
		return file, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kindest.yaml"), nil
}

func resolveDockerfile(rootDir string, dockerfilePath string, contextPath string) (string, error) {
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	dockerfilePath = filepath.Clean(filepath.Join(rootDir, dockerfilePath))
	contextPath = filepath.Clean(filepath.Join(rootDir, contextPath))
	rel, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil {
		return "", err
	}
	// On Windows, the dockerfile path has to be converted to forward slashes
	return filepath.ToSlash(rel), nil
}

func loadSpec(file string, log logger.Logger) (*KindestSpec, string, error) {
	manifestPath, err := locateSpec(file)
	if err != nil {
		return nil, "", err
	}
	docBytes, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, "", err
	}
	spec := &KindestSpec{}
	if err := yaml.Unmarshal(docBytes, spec); err != nil {
		return nil, "", err
	}
	if err := spec.Verify(manifestPath, log); err != nil {
		return nil, "", err
	}
	return spec, manifestPath, nil
}

func getAuthConfig(domain string, configs map[string]types.AuthConfig) (*types.AuthConfig, error) {
	for name, config := range configs {
		if strings.Contains(name, domain) {
			return &config, nil
		}
	}
	return &types.AuthConfig{}, nil
}

func RegistryAuthFromEnv(imageName string) (*types.AuthConfig, error) {
	named, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return nil, err
	}
	domain := reference.Domain(named)
	cf := configfile.New("")
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(u.HomeDir, ".docker", "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := cf.LoadFromReader(f); err != nil {
		return nil, err
	}
	return getAuthConfig(domain, cf.GetAuthConfigs())
}
