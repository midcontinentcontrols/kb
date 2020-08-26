package kindest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Jeffail/tunny"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/hashicorp/go-multierror"

	"go.uber.org/zap"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/monochromegane/go-gitignore"
)

type BuildStatus int32

func (b BuildStatus) String() string {
	switch b {
	case BuildStatusPending:
		return "Pending"
	case BuildStatusInProgress:
		return "InProgress"
	case BuildStatusFailed:
		return "Failed"
	case BuildStatusSucceeded:
		return "Succeeded"
	default:
		return fmt.Sprintf("Unknown (%d)", int32(b))
	}
}

const (
	BuildStatusPending    BuildStatus = 0
	BuildStatusInProgress BuildStatus = 1
	BuildStatusFailed     BuildStatus = 2
	BuildStatusSucceeded  BuildStatus = 3
)

type resolver struct {
	l       sync.Mutex         //
	modules map[string]*Module // map of manifestPath to *Module
}

type Module struct {
	Spec         *KindestSpec
	Dir          string
	Dependencies []*Module //
	status       int32
	subscribersL sync.Mutex
	subscribers  []chan<- error
	err          unsafe.Pointer
	log          logger.Logger
	pool         *tunny.Pool
}

var ErrModuleNotCached = fmt.Errorf("module is not cached")

func (m *Module) CachedDigest() (string, error) {
	path, err := digestPathForManifest(m.Dir)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadFile(path)
	if err != nil {
		return "", ErrModuleNotCached
	}
	return string(body), nil
}

func (m *Module) cacheDigest(digest string) error {
	path, err := digestPathForManifest(m.Dir)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(path, []byte(digest), 0644); err != nil {
		return err
	}
	return nil
}

func collectErrors(dones []chan error) error {
	var multi error
	for _, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, err)
		}
	}
	return multi
}

func (m *Module) buildDependencies(options *BuildOptions) error {
	n := len(m.Dependencies)
	dones := make([]chan error, n, n)
	for i, dependency := range m.Dependencies {
		done := make(chan error, 1)
		dones[i] = done
		go func(dependency *Module, done chan<- error) {
			defer close(done)
			err, _ := m.pool.Process(&buildJob{
				m:       dependency,
				options: options,
			}).(error)
			if err != nil {
				err = fmt.Errorf("%s: %v", dependency.Dir, err)
			}
			done <- err
		}(dependency, done)
	}
	return collectErrors(dones)
}

func addFileToBuildContext(
	dir string,
	relativePath string,
	c map[string]Entity,
) error {
	parts := strings.Split(relativePath, string(os.PathSeparator))
	var e Entity
	var ok bool
	filePath := dir
	for i, part := range parts {
		filePath = filepath.Join(filePath, part)
		e, ok = c[part]
		if ok {
			if i < len(parts)-1 {
				d := e.(*Directory)
				c = d.Contents
			} else {
				// Already added!
				return nil
			}
		} else {
			info, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("failed to stat '%s': %v", filePath, err)
			}
			if i < len(parts)-1 {
				d := &Directory{
					info:     info,
					Contents: map[string]Entity{},
				}
				c[part] = d
				c = d.Contents
			} else {
				body, err := ioutil.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("failed to read file: %v", err)
				}
				c[part] = &File{
					info:    info,
					Content: body,
				}
				return nil
			}
		}
	}
	return fmt.Errorf("failed to add file")
}

func addDirToBuildContext(
	dir string,
	contextPath string,
	dockerignore gitignore.IgnoreMatcher,
	include gitignore.IgnoreMatcher,
	c map[string]Entity,
) error {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		name := info.Name()
		path := filepath.Join(dir, name)
		rel, err := filepath.Rel(contextPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		isDir := info.IsDir()
		excludeFile := dockerignore.Match(rel, isDir) // || !include.Match(rel, isDir)
		if excludeFile {
			continue
		} else {
			if info.IsDir() {
				contents := make(map[string]Entity)
				if err := addDirToBuildContext(
					path,
					contextPath,
					dockerignore,
					include,
					contents,
				); err != nil {
					return err
				}
				c[name] = &Directory{
					Contents: contents,
					info:     info,
				}
			} else {
				body, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				c[name] = &File{
					Content: body,
					info:    info,
				}
			}
		}
	}
	return nil
}

func createDockerInclude(contextPath string, dockerfilePath string) (gitignore.IgnoreMatcher, error) {
	f, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var addedPaths []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "COPY") || strings.HasPrefix(line, "ADD") {
			fields := strings.Fields(line)
			if rel := fields[1]; !strings.HasPrefix(rel, "--from") {
				abs := filepath.Clean(filepath.Join(contextPath, rel))
				info, err := os.Stat(abs)
				if err != nil {
					return nil, fmt.Errorf("failed to stat %v", abs)
				}
				if info.IsDir() && !strings.HasSuffix(rel, "/") {
					rel += "/"
				}
				parts := strings.Split(rel, "/")
				for i := range parts {
					var full string
					for _, other := range parts[:i+1] {
						full = filepath.Join(full, other)
					}
					found := false
					for _, item := range addedPaths {
						if item == full {
							found = true
							break
						}
					}
					if !found {
						addedPaths = append(addedPaths, full)
					}
				}
			}
		}
	}
	return gitignore.NewGitIgnoreFromReader(
		"",
		bytes.NewBuffer([]byte(strings.Join(addedPaths, "\n"))),
	), nil
}

func getRelativeDockerfilePath(contextPath, dockerfilePath string) (string, error) {
	relativeDockerfile, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil {
		return "", err
	}
	relativeDockerfile = filepath.ToSlash(relativeDockerfile)
	return relativeDockerfile, nil
}

func (m *Module) loadBuildContext() (BuildContext, string, gitignore.IgnoreMatcher, error) {
	contextPath := filepath.Clean(filepath.Join(m.Dir, m.Spec.Build.Context))
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	var dockerignore gitignore.IgnoreMatcher
	if _, err := os.Stat(dockerignorePath); err == nil {
		r, err := os.Open(dockerignorePath)
		if err != nil {
			return nil, "", nil, err
		}
		defer r.Close()
		dockerignore = gitignore.NewGitIgnoreFromReader("", r)
	} else {
		dockerignore = gitignore.NewGitIgnoreFromReader("", bytes.NewReader([]byte("")))
	}
	dockerfilePath := m.Spec.Build.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	dockerfilePath = filepath.Clean(filepath.Join(m.Dir, dockerfilePath))
	relativeDockerfile, err := getRelativeDockerfilePath(contextPath, dockerfilePath)
	if err != nil {
		return nil, "", nil, err
	}
	include, err := createDockerInclude(contextPath, dockerfilePath)
	if err != nil {
		return nil, "", nil, err
	}
	c := make(map[string]Entity)
	if err := addDirToBuildContext(
		contextPath,
		contextPath,
		dockerignore,
		include,
		c,
	); err != nil {
		return nil, "", nil, err
	}
	if _, ok := c[".git"]; ok {
		panic(".git was included")
	}
	if err := addFileToBuildContext(
		contextPath,
		relativeDockerfile,
		c,
	); err != nil {
		return nil, "", nil, err
	}
	//printBuildContext(c, 0)
	return BuildContext(c), relativeDockerfile, include, nil
}

func printBuildContext(c map[string]Entity, indent int) {
	print := func(msg string, args ...interface{}) {
		for i := 0; i < indent; i++ {
			fmt.Fprintf(os.Stdout, "\t")
		}
		fmt.Fprintf(os.Stdout, msg, args...)
	}
	for k, v := range c {
		if d, ok := v.(*Directory); ok {
			print("%s\n", k)
			printBuildContext(d.Contents, indent+1)
		} else if f, ok := v.(*File); ok {
			print("%s: %d bytes\n", k, len(f.Content))
		} else {
			panic("unreachable branch detected")
		}
	}
}

func (m *Module) Status() BuildStatus {
	return BuildStatus(atomic.LoadInt32(&m.status))
}

func (m *Module) setStatus(status BuildStatus) {
	atomic.StoreInt32(&m.status, int32(status))
}

func (m *Module) claim() bool {
	return atomic.CompareAndSwapInt32(
		&m.status,
		int32(BuildStatusPending),
		int32(BuildStatusInProgress),
	)
}

func (m *Module) subscribe(done chan<- error) {
	m.subscribersL.Lock()
	defer m.subscribersL.Unlock()
	switch m.Status() {
	case BuildStatusInProgress:
		m.subscribers = append(m.subscribers, done)
	case BuildStatusFailed:
		// m.err may be nil because of threading volatility
		box := (*string)(atomic.LoadPointer(&m.err))
		done <- fmt.Errorf(*box)
	case BuildStatusSucceeded:
		done <- nil
	default:
		panic("unreachable")
	}
}

func (m *Module) broadcast(err error) {
	m.subscribersL.Lock()
	defer m.subscribersL.Unlock()
	if err != nil {
		msg := err.Error()
		m.setStatus(BuildStatusFailed)
		atomic.StorePointer(&m.err, unsafe.Pointer(&msg))
	} else {
		m.setStatus(BuildStatusSucceeded)
	}
	for _, subscriber := range m.subscribers {
		subscriber <- err
		close(subscriber)
	}
}

func (m *Module) WaitForCompletion() error {
	done := make(chan error, 1)
	m.subscribe(done)
	return <-done
}

func buildDocker(
	m *Module,
	buildContext []byte,
	resolvedDockerfile string,
	options *BuildOptions,
) error {
	dest := sanitizeImageName(options.Repository, m.Spec.Build.Name, options.Tag)
	log := m.log.With(zap.String("dest", dest))
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	buildArgs := make(map[string]*string)
	for _, arg := range m.Spec.Build.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resp, err := cli.ImageBuild(
		context.TODO(),
		bytes.NewReader(buildContext),
		dockertypes.ImageBuildOptions{
			NoCache:    options.NoCache,
			Dockerfile: resolvedDockerfile,
			BuildArgs:  buildArgs,
			Squash:     options.Squash,
			Tags:       []string{dest},
			Target:     m.Spec.Build.Target,
		},
	)
	if err != nil {
		return err
	}
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
	if !options.NoPush {
		authConfig, err := RegistryAuthFromEnv(dest)
		if err != nil {
			return err
		}
		log.Info(
			"Pushing image",
			zap.String("username", string(authConfig.Username)),
		)
		authBytes, err := json.Marshal(authConfig)
		if err != nil {
			return err
		}
		registryAuth := base64.URLEncoding.EncodeToString(authBytes)
		resp, err := cli.ImagePush(
			context.TODO(),
			dest,
			dockertypes.ImagePushOptions{
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
	}
	log.Info("Successfully built image", zap.Bool("noPush", options.NoPush))
	return nil
}

func buildKaniko(
	m *Module,
	buildContext []byte,
	resolvedDockerfile string,
	options *BuildOptions,
) error {
	return fmt.Errorf("kaniko builder unimplemented")
}

func doBuild(
	m *Module,
	buildContext []byte,
	resolvedDockerfile string,
	options *BuildOptions,
) error {
	switch options.Builder {
	case "":
		fallthrough
	case "docker":
		if err := buildDocker(
			m,
			buildContext,
			resolvedDockerfile,
			options,
		); err != nil {
			return fmt.Errorf("docker: %v", err)
		}
	case "kaniko":
		if err := buildKaniko(
			m,
			buildContext,
			resolvedDockerfile,
			options,
		); err != nil {
			return fmt.Errorf("kaniko: %v", err)
		}
	default:
		return fmt.Errorf("unknown builder '%s'", options.Builder)
	}
	return nil
}

func (m *Module) Build(options *BuildOptions) (err error) {
	if !m.claim() {
		switch m.Status() {
		case BuildStatusInProgress:
			return m.WaitForCompletion()
		case BuildStatusFailed:
			box := (*string)(atomic.LoadPointer(&m.err))
			if box == nil {
				panic("unreachable")
			}
			return fmt.Errorf(*box)
		case BuildStatusSucceeded:
			return nil
		default:
			panic("unreachable")
		}
	}
	defer func() {
		// Inform all subscribers of return value
		m.broadcast(err)
	}()
	if err := m.buildDependencies(options); err != nil {
		return err
	}
	if m.Spec.Build == nil {
		return nil
	}
	if !options.SkipHooks {
		if err := runCommands(m.Spec.Build.Before); err != nil {
			return fmt.Errorf("pre-build hook failure: %v", err)
		}
	}
	// Create a docker "include" that lists files included by the build context.
	// This is necessary for calculating the digest
	buildContext, resolvedDockerfile, include, err := m.loadBuildContext()
	if err != nil {
		return err
	}
	digest, err := buildContext.Digest(include)
	if err != nil {
		return err
	}
	cachedDigest, err := m.CachedDigest()
	if err != nil && err != ErrModuleNotCached {
		return err
	}
	if digest == cachedDigest && !options.NoCache {
		m.log.Info("No files changed", zap.String("digest", cachedDigest))
		return nil
	}
	tar, err := buildContext.Archive()
	if err != nil {
		return err
	}
	if err := doBuild(
		m,
		tar,
		resolvedDockerfile,
		options,
	); err != nil {
		return err
	}
	if err := m.cacheDigest(digest); err != nil {
		return err
	}
	if !options.SkipHooks {
		if err := runCommands(m.Spec.Build.After); err != nil {
			return fmt.Errorf("post-build hook failure: %v", err)
		}
	}
	return nil
}

func runCommands(commands []string) error {
	for _, command := range commands {
		cmd := exec.Command("sh", "-c", command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
