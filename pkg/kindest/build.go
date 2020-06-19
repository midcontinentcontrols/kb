package kindest

import (
	"archive/tar"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Jeffail/tunny"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/distribution/reference"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/jhoonb/archivex"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/monochromegane/go-gitignore"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/kind/pkg/cluster"
)

type BuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type BuildSpec struct {
	Name       string      `json:"name"`
	Dockerfile string      `json:"dockerfile,omitempty" yaml:"dockerfile,omitempty"`
	Context    string      `json:"context,omitempty" yaml:"context,omitempty"`
	BuildArgs  []*BuildArg `json:"buildArgs,omitempty" yaml:"buildArgs,omitempty"`
	Target     string      `json:"target,omitempty" yaml:"target,omitempty"`
	Command    []string    `json:"command,omitempty" yaml:"command,omitempty"`
}

func (b *BuildSpec) verifyDocker(manifestPath string, log logger.Logger) error {
	var path string
	if b.Dockerfile != "" {
		path = filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(b.Dockerfile))
	} else {
		path = filepath.Join(filepath.Dir(manifestPath), "Dockerfile")
	}
	path = filepath.Clean(path)
	log.Info("Resolving Dockerfile", zap.String("name", b.Name), zap.String("path", path))
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

func hashDir(
	dir string,
	contextPath string,
	dockerignore gitignore.IgnoreMatcher,
	h hash.Hash,
	resolvedDockerfile string,
) error {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		path := filepath.Join(dir, info.Name())
		rel, err := filepath.Rel(contextPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// Always include the specific Dockerfile in the build context,
		// regardless of what .dockerignore says. It's something sneaky
		// that `docker build` does.
		if rel != resolvedDockerfile && dockerignore.Match(rel, info.IsDir()) {
			continue
		} else {
			if info.IsDir() {
				// mangle names of folders from files so an empty file
				// and empty folder do not have the same hash
				if _, err := h.Write([]byte(fmt.Sprintf("%s?f", rel))); err != nil {
					return err
				}
				if err := hashDir(path, contextPath, dockerignore, h, resolvedDockerfile); err != nil {
					return err
				}
			} else {
				if _, err := h.Write([]byte(rel)); err != nil {
					return err
				}
				body, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				if _, err := h.Write(body); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func tarDir(
	dir string,
	contextPath string,
	dockerignore gitignore.IgnoreMatcher,
	archive *archivex.TarFile,
	resolvedDockerfile string,
) error {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		path := filepath.Join(dir, info.Name())
		rel, err := filepath.Rel(contextPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		//log.Info("Matching", zap.String("rel", rel), zap.Bool("isDir", info.IsDir()))
		// Always include the specific Dockerfile in the build context,
		// regardless of what .dockerignore says. It's something sneaky
		// that `docker build` does.
		if rel != resolvedDockerfile && dockerignore.Match(rel, info.IsDir()) {
			continue
		} else {
			//log.Info("Should not ignore", zap.String("rel", rel), zap.Bool("isDir", info.IsDir()))
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = rel
			if info.IsDir() {
				header.Name += "/"
				if err := archive.Writer.WriteHeader(header); err != nil {
					return err
				}
				if err := tarDir(path, contextPath, dockerignore, archive, resolvedDockerfile); err != nil {
					return err
				}
			} else {
				//log.Info("Adding file to build context", zap.String("rel", rel), zap.String("abs", path))
				if err := archive.Writer.WriteHeader(header); err != nil {
					return err
				}
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				n, err := io.Copy(archive.Writer, f)
				if err != nil {
					f.Close()
					return err
				}
				if err := f.Close(); err != nil {
					return err
				}
				if n != info.Size() {
					return fmt.Errorf("unexpected amount of bytes copied")
				}
			}
		}
	}
	return nil
}

func (b *BuildSpec) hashBuildContext(manifestPath string, options *BuildOptions) (string, error) {
	contextPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(b.Context)))
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		b.Dockerfile,
		b.Context,
	)
	if err != nil {
		return "", err
	}
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	h := md5.New()
	if _, err := os.Stat(dockerignorePath); err == nil {
		r, err := os.Open(dockerignorePath)
		if err != nil {
			return "", err
		}
		defer r.Close()
		dockerignore := gitignore.NewGitIgnoreFromReader("", r)
		if err != nil {
			return "", err
		}
		if err := hashDir(
			contextPath,
			contextPath,
			dockerignore,
			h,
			resolvedDockerfile,
		); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (b *BuildSpec) tarBuildContext(manifestPath string, options *BuildOptions) (string, error) {
	contextPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(b.Context)))
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	tmpDir := filepath.Join(u.HomeDir, ".kindest", "tmp")
	if err := os.MkdirAll(tmpDir, 0766); err != nil {
		return "", err
	}
	tarPath := filepath.Join(tmpDir, fmt.Sprintf("build-context-%s.tar", uuid.New().String()))
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		b.Dockerfile,
		b.Context,
	)
	if err != nil {
		return "", err
	}
	archive := new(archivex.TarFile)
	archive.Create(tarPath)
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	if _, err := os.Stat(dockerignorePath); err == nil {
		r, err := os.Open(dockerignorePath)
		if err != nil {
			return "", err
		}
		defer r.Close()
		dockerignore := gitignore.NewGitIgnoreFromReader("", r)
		if err != nil {
			return "", err
		}
		if err := tarDir(
			contextPath,
			contextPath,
			dockerignore,
			archive,
			resolvedDockerfile,
		); err != nil {
			return "", err
		}
	} else if err := archive.AddAll(contextPath, false); err != nil {
		return "", err
	}
	if err := archive.Close(); err != nil {
		return "", err
	}
	return tarPath, nil
}

func (b *BuildSpec) buildDocker(
	manifestPath string,
	options *BuildOptions,
	respHandler func(io.ReadCloser) error,
	log logger.Logger,
) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	tmpDir := filepath.Join(u.HomeDir, ".kindest", "tmp")
	if err := os.MkdirAll(tmpDir, 0766); err != nil {
		return err
	}
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		b.Dockerfile,
		b.Context,
	)
	if err != nil {
		return err
	}
	dest := sanitizeImageName(options.Repository, b.Name, options.Tag)
	log.Info("Building",
		zap.String("builder", "docker"),
		zap.String("resolvedDockerfile", resolvedDockerfile),
		zap.String("dest", dest),
		zap.Bool("noPush", options.NoPush),
		zap.Bool("noCache", options.NoCache))
	tarPath, err := b.tarBuildContext(manifestPath, options)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)
	dockerBuildContext, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()
	buildArgs := make(map[string]*string)
	for _, arg := range b.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resp, err := cli.ImageBuild(
		context.TODO(),
		dockerBuildContext,
		dockertypes.ImageBuildOptions{
			NoCache:    options.NoCache,
			Dockerfile: resolvedDockerfile,
			BuildArgs:  buildArgs,
			Squash:     options.Squash,
			Tags:       []string{dest},
			Target:     b.Target,
		},
	)
	if err != nil {
		return err
	}
	if respHandler != nil {
		if err := respHandler(resp.Body); err != nil {
			return err
		}
	} else {
		termFd, isTerm := term.GetFdInfo(os.Stderr)
		if err := jsonmessage.DisplayJSONMessagesStream(
			resp.Body,
			os.Stderr,
			termFd,
			isTerm,
			nil,
		); err != nil {
			return err
		}
	}
	if !options.NoPush {
		log := log.With(zap.String("dest", dest))
		log.Info("Pushing image")
		authConfig, err := RegistryAuthFromEnv(dest)
		if err != nil {
			return err
		}
		log.Info("Using docker credentials from env",
			zap.String("username", string(authConfig.Username)))
		authBytes, err := json.Marshal(authConfig)
		if err != nil {
			return err
		}
		registryAuth := base64.URLEncoding.EncodeToString(authBytes)
		resp, err := cli.ImagePush(
			context.TODO(),
			dest,
			dockertypes.ImagePushOptions{
				All:          false,
				RegistryAuth: registryAuth,
			},
		)
		if err != nil {
			return err
		}
		termFd, isTerm := term.GetFdInfo(os.Stderr)
		if err := jsonmessage.DisplayJSONMessagesStream(
			resp,
			os.Stderr,
			termFd,
			isTerm,
			nil,
		); err != nil {
			return err
		}
		log.Info("Pushed image")
	}
	if options.Kind != "" {
		log.Info("Copying image to kind cluster", zap.String("name", options.Kind))
		if err := loadImageOnCluster(
			dest,
			options.Kind,
			cluster.NewProvider(),
		); err != nil {
			return err
		}
	}
	return nil
}

var errDigestNotCached = fmt.Errorf("digest not cached")

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

func (b *BuildSpec) loadCachedDigest(manifestPath string) (string, error) {
	path, err := digestPathForManifest(manifestPath)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadFile(path)
	if err != nil {
		// TODO: disambiguate error and fix this
		return "", errDigestNotCached
	}
	return string(body), errDigestNotCached
}

func (b *BuildSpec) cacheDigest(manifestPath string, value string) error {
	path, err := digestPathForManifest(manifestPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(path, []byte(value), 0644); err != nil {
		return err
	}
	return nil
}

func (b *BuildSpec) Build(
	manifestPath string,
	options *BuildOptions,
	respHandler func(io.ReadCloser) error,
	log logger.Logger,
) error {
	digest, err := b.hashBuildContext(manifestPath, options)
	if err != nil {
		return err
	}
	if !options.NoCache && !options.Force {
		// Check to see if files actually changed
		cachedDigest, err := b.loadCachedDigest(manifestPath)
		if err != nil && err != errDigestNotCached {
			return err
		}
		if digest == cachedDigest {
			log.Debug("No files changed", zap.String("digest", digest))
			return nil
		}
	} else {
		log.Debug("Bypassing cache")
	}
	switch options.Builder {
	case "kaniko":
		err = b.buildKaniko(
			manifestPath,
			options,
			log,
		)
	case "":
		fallthrough
	case "docker":
		err = b.buildDocker(
			manifestPath,
			options,
			respHandler,
			log,
		)
	default:
		return fmt.Errorf("unknown builder '%s'", options.Builder)
	}
	if err != nil {
		return err
	}
	if err := b.cacheDigest(manifestPath, digest); err != nil {
		return err
	}
	log.Debug("Updated cache", zap.String("digest", digest))
	return nil
}

type BuildOptions struct {
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	NoCache     bool   `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag         string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Concurrency int    `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	Context     string `json:"context,omitempty" yaml:"context,omitempty"`
	Repository  string `json:"repository,omitempty" yaml:"repository,omitempty"`
	Builder     string `json:"builder,omitempty" yaml:"builder,omitempty"`
	NoPush      bool   `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	Kind        string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Force       bool   `json:"force,omitempty"` // If true, will always run docker build regardless of kindest digest
}

func buildDependencies(
	spec *KindestSpec,
	manifestPath string,
	options *BuildOptions,
	pool *tunny.Pool,
) error {
	n := len(spec.Dependencies)
	dones := make([]chan error, n, n)
	rootDir := filepath.Dir(manifestPath)
	for i, dep := range spec.Dependencies {
		done := make(chan error, 1)
		dones[i] = done
		go func(dep string, done chan<- error) {
			opts := &BuildOptions{}
			*opts = *options
			opts.File = filepath.Clean(filepath.Join(rootDir, dep, "kindest.yaml"))
			err, _ := pool.Process(opts).(error)
			done <- err
			close(done)
		}(dep, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("dependency '%s': %v", spec.Dependencies[i], err))
		}
	}
	return multi
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

func resolveDockerfile(manifestPath string, dockerfilePath string, contextPath string) (string, error) {
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	rootDir := filepath.Dir(manifestPath)
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
	if err := spec.Validate(manifestPath, log); err != nil {
		return nil, "", err
	}
	return spec, manifestPath, nil
}

func Build(options *BuildOptions, log logger.Logger) error {
	var pool *tunny.Pool
	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = runtime.NumCPU()
	}
	pool = tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		options := payload.(*BuildOptions)
		return BuildEx(options, pool, nil, nil, log)
	})
	defer pool.Close()
	return BuildEx(options, pool, nil, nil, log)
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

func BuildEx(
	options *BuildOptions,
	pool *tunny.Pool,
	respHandler func(io.ReadCloser) error,
	images chan<- string,
	log logger.Logger,
) error {
	spec, manifestPath, err := loadSpec(options.File, log)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	if err := buildDependencies(
		spec,
		manifestPath,
		options,
		pool,
	); err != nil {
		return err
	}
	if spec.Build != nil {
		if err := spec.Build.Build(
			manifestPath,
			options,
			respHandler,
			log,
		); err != nil {
			return err
		}
		if images != nil {
			images <- sanitizeImageName(options.Repository, spec.Build.Name, options.Tag)
		}
	}
	return nil
}

/*
func detectErrorMessage(in io.Reader) error {
	dec := json.NewDecoder(in)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		log.Info("Push", zap.String("message", fmt.Sprintf("%+v", jm)))
		// skip progress message
		//if jm.Progress == nil {
		//glog.Infof("%v", jm)
		//}
		if jm.Error != nil {
			return jm.Error
		}

		if len(jm.ErrorMessage) > 0 {
			return errors.New(jm.ErrorMessage)
		}
	}
	return nil
}
*/
