package kindest

import (
	"bufio"
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
	"github.com/hashicorp/go-multierror"
	gogitignore "github.com/sabhiram/go-gitignore"

	"go.uber.org/zap"

	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
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
	Path         string
	Dependencies []*Module //
	status       int32
	subscribersL sync.Mutex
	subscribers  []chan<- error
	err          unsafe.Pointer
	log          logger.Logger
	pool         *tunny.Pool
	builtImagesL sync.Mutex
	BuiltImages  []string
	p            *Process
}

func (m *Module) ListImages() ([]string, error) {
	var images []string
	for _, dep := range m.Dependencies {
		depImages, err := dep.ListImages()
		if err != nil {
			return nil, fmt.Errorf("dependency '%s': %v", dep.Path, err)
		}
		images = append(images, depImages...)
	}
	if m.Spec.Build != nil {
		images = append(images, m.Spec.Build.Name)
	}
	return images, nil
}

func (m *Module) Dir() string {
	return filepath.Dir(m.Path)
}

func (m *Module) builtImage(imageName string) {
	m.builtImagesL.Lock()
	defer m.builtImagesL.Unlock()
	found := false
	for _, image := range m.BuiltImages {
		if image == imageName {
			found = true
			break
		}
	}
	if !found {
		m.BuiltImages = append(m.BuiltImages, imageName)
	}
}

var ErrModuleNotCached = fmt.Errorf("module is not cached")

func (m *Module) RunTests2(options *TestOptions, log logger.Logger) error {
	if !options.SkipBuild {
		if options.Kind != "" {
			var err error
			options.KubeContext, err = cluster_management.CreateCluster(options.Kind, log)
			if err != nil {
				return err
			}
		}
		if err := m.Build(&options.BuildOptions); err != nil {
			return err
		}
	}
	if !options.SkipDeploy {
		if _, err := m.Deploy(&DeployOptions{
			Kind:          options.Kind,
			KubeContext:   options.KubeContext,
			Repository:    options.Repository,
			Tag:           options.Tag,
			Verbose:       options.Verbose,
			RestartImages: m.BuiltImages,
			Wait:          true,
		}); err != nil {
			return err
		}
	}
	if err := m.RunTests(options, log); err != nil {
		return err
	}
	return nil
}

func (m *Module) RunTests(options *TestOptions, log logger.Logger) error {
	return m.Spec.RunTests(options, m.Path, m.p, log)
}

func (m *Module) CachedDigest(imageName string) (string, error) {
	path, err := digestPathForManifest(imageName)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadFile(path)
	if err != nil {
		return "", ErrModuleNotCached
	}
	return string(body), nil
}

func (m *Module) cacheDigest(imageName string, digest string) error {
	path, err := digestPathForManifest(imageName)
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
	dones := make([]chan error, n)
	for i, dependency := range m.Dependencies {
		done := make(chan error, 1)
		dones[i] = done
		go func(dependency *Module, done chan<- error) {
			err := dependency.Build(options)
			if err != nil {
				err = fmt.Errorf("%s: %v", dependency.Path, err)
			} else {
				for _, dest := range dependency.BuiltImages {
					m.builtImage(dest)
				}
			}
			done <- err
			close(done)
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
	dockerignore *gogitignore.GitIgnore,
	include *gogitignore.GitIgnore,
	traverse []string,
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
		if info.IsDir() {
			if !strings.HasSuffix(rel, "/") {
				rel += "/"
			}
		}
		excludeFile := dockerignore.MatchesPath(rel)
		if excludeFile {
			continue
		}
		if info.IsDir() {
			// How do we know if we need to traverse a directory?
			// Check if rel is included in the list of directories
			// to traverse. If it is, we'll need to create it in
			// the BuildContext structure.
			if !include.MatchesPath(rel) {
				found := false
				withoutTrailingSlash := rel[:len(rel)-1]
				for _, dir := range traverse {
					if dir == withoutTrailingSlash {
						found = true
						break
					}
				}
				if !found {
					// This directory is not relevant to the build
					continue
				}
			}
			contents := make(map[string]Entity)
			if err := addDirToBuildContext(
				path,
				contextPath,
				dockerignore,
				include,
				traverse,
				contents,
			); err != nil {
				return err
			}
			c[name] = &Directory{
				Contents: contents,
				info:     info,
			}
		} else if include.MatchesPath(rel) {
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
	return nil
}

func createDockerInclude(contextPath string, dockerfilePath string) (*gogitignore.GitIgnore, []string, error) {
	f, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var addedPaths []string
	var traverse []string
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
					return nil, nil, fmt.Errorf("failed to stat %v: %v", abs, err)
				}
				if info.IsDir() && !strings.HasSuffix(rel, "/") {
					rel += "/"
				}

				// Add it to the explicit dockerinclude
				found := false
				for _, item := range addedPaths {
					if item == rel {
						found = true
						break
					}
				}
				if !found {
					if rel == "./" {
						// Fix issue where `COPY . .` doesn't work by
						// adding all files to the include matcher.
						rel = "*"
					}
					addedPaths = append(addedPaths, rel)
				}

				// Add all the necessary parent paths to the traversal list
				parts := strings.Split(rel, "/")
				for i := range parts[:len(parts)-1] {
					if parts[i] == "" {
						// Trailing slash
						break
					}
					var full string
					for _, other := range parts[:i+1] {
						if full != "" {
							full += "/"
						}
						full += other
					}
					found := false
					for _, item := range traverse {
						if item == full {
							found = true
							break
						}
					}
					if !found {
						//fmt.Printf("Added %s\n", full)
						traverse = append(traverse, full)
					}
				}
			}
		}
	}
	return gogitignore.CompileIgnoreLines(addedPaths...), traverse, nil
}

func getRelativeDockerfilePath(contextPath, dockerfilePath string) (string, error) {
	relativeDockerfile, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil {
		return "", err
	}
	relativeDockerfile = filepath.ToSlash(relativeDockerfile)
	return relativeDockerfile, nil
}

func (m *Module) loadBuildContext() (BuildContext, string, *gogitignore.GitIgnore, error) {
	contextPath := filepath.Clean(filepath.Join(m.Dir(), m.Spec.Build.Context))
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	var dockerignore *gogitignore.GitIgnore
	if _, err := os.Stat(dockerignorePath); err == nil {
		dockerignore, err = gogitignore.CompileIgnoreFile(dockerignorePath)
		if err != nil {
			return nil, "", nil, fmt.Errorf("gogitignore: %v", err)
		}
	} else {
		dockerignore = gogitignore.CompileIgnoreLines()
	}
	dockerfilePath := m.Spec.Build.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	dockerfilePath = filepath.Clean(filepath.Join(m.Dir(), dockerfilePath))
	relativeDockerfile, err := getRelativeDockerfilePath(contextPath, dockerfilePath)
	if err != nil {
		return nil, "", nil, err
	}
	include, traverse, err := createDockerInclude(contextPath, dockerfilePath)
	if err != nil {
		return nil, "", nil, err
	}
	c := make(map[string]Entity)
	if err := addDirToBuildContext(
		contextPath,
		contextPath,
		dockerignore,
		include,
		traverse,
		c,
	); err != nil {
		return nil, "", nil, err
	}
	if _, ok := c[".git"]; ok {
		m.log.Warn(
			".git was included in the build context, which may not be intentional",
			zap.String("contextPath", contextPath),
			zap.Bool("dockerignore.MatchesPath('.git/')", dockerignore.MatchesPath(".git/")),
		)
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

/*
func copyDockerCredential(
	client *kubernetes.Clientset,
	config *restclient.Config,
	pod *corev1.Pod,
) error {
	var dockerconfigjson string
	home := util.HomeDir()
	if home == "" {
		home = "/root"
	}
	body, err := ioutil.ReadFile(filepath.Join(home, ".docker", "config.json"))
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	dockerconfigjson = string(body)
	if err := execInPod(
		client,
		config,
		pod,
		&corev1.PodExecOptions{
			Command: []string{
				"sh",
				"-c",
				fmt.Sprintf("echo '%s' > /kaniko/.docker/config.json", dockerconfigjson),
			},
			Stdin:  false,
			Stdout: true,
			Stderr: true,
			TTY:    false,
		},
		nil,
		os.Stdout,
		os.Stderr,
	); err != nil {
		return err
	}
	return nil
}


*/

func doBuildModule(
	spec *BuildSpec,
	buildContext []byte,
	relativeDockerfile string,
	options *BuildOptions,
	log logger.Logger,
) error {
	dest := util.SanitizeImageName(options.Repository, spec.Name, options.Tag)
	log = log.With(zap.String("dest", dest))
	switch options.Builder {
	case "":
		fallthrough
	case "docker":
		if err := buildDocker(
			spec,
			dest,
			buildContext,
			relativeDockerfile,
			options,
			log,
		); err != nil {
			return err
		}
	case "kaniko":
		if err := buildKaniko(
			spec,
			dest,
			buildContext,
			relativeDockerfile,
			options,
			log,
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown builder '%s'", options.Builder)
	}
	return nil
}

func (m *Module) GetAffectedModules(files []string) ([]*Module, error) {
	var affected []*Module
	for _, dep := range m.Dependencies {
		dependentAffected, err := dep.GetAffectedModules(files)
		if err != nil {
			return nil, err
		}
		for _, dependent := range dependentAffected {
			found := false
			for _, other := range affected {
				if dependent == other {
					found = true
					break
				}
			}
			if !found {
				affected = append(affected, dependent)
			}
		}
	}
	// TODO: there is a situation where a module will
	// require rebuilding without any of its files
	// being affected, such as when it uses a base image
	// that is affected by the files changed.
	depends, err := m.DependsOnFiles(files)
	if err != nil {
		return nil, err
	}
	if depends {
		affected = append(affected, m)
	} else if m.Spec.Build != nil {
		baseImage, err := m.Spec.Build.GetBaseImage(m.Dir())
		if err != nil {
			return nil, err
		}
		// ignore tag for now
		parts := strings.Split(baseImage, ":")
		baseImage = parts[0]
		for _, dep := range affected {
			affectedImages, err := dep.ListImages()
			if err != nil {
				return nil, fmt.Errorf("ListImages: %v", err)
			}
			for _, affectedImage := range affectedImages {
				//fmt.Printf("Checking %s == %s, %v\n", affectedImage, baseImage, affectedImage == baseImage)
				if affectedImage == baseImage {
					// Affected module builds a base image
					affected = append(affected, m)
					break
				}
			}
		}
	}
	return affected, nil
}

func (m *Module) DependsOnFiles(files []string) (bool, error) {
	if m.Spec.Build == nil {
		return false, nil
	}
	return m.Spec.Build.DependsOnFiles(files, m.Path)
}

func (m *Module) doBuild(options *BuildOptions) error {
	if !options.SkipHooks {
		if err := runCommands(m.Spec.Build.Before); err != nil {
			return fmt.Errorf("pre-build hook failure: %v", err)
		}
	}
	// Create a docker "include" that lists files included by the build context.
	// This is necessary for calculating the digest
	buildContext, relativeDockerfile, include, err := m.loadBuildContext()
	if err != nil {
		return err
	}
	digest, err := buildContext.Digest(include)
	if err != nil {
		return err
	}
	dest := util.SanitizeImageName(options.Repository, m.Spec.Build.Name, options.Tag)
	cachedDigest, err := m.CachedDigest(dest)
	if err != nil && err != ErrModuleNotCached {
		return err
	}
	if digest == cachedDigest && !options.NoCache && !options.Force {
		m.log.Debug("No files changed", zap.String("digest", cachedDigest))
		return nil
	}
	if digest != cachedDigest {
		m.log.Debug(
			"Digests do not match, building...",
			zap.String("module", m.Path))
	}
	tar, err := buildContext.Archive()
	if err != nil {
		return err
	}
	if err := doBuildModule(
		m.Spec.Build,
		tar,
		relativeDockerfile,
		options,
		m.log,
	); err != nil {
		return err
	}
	m.log.Info(
		"Successfully built image",
		zap.String("dest", dest),
		zap.String("digest", digest),
		zap.Bool("noPush", options.NoPush))
	m.builtImage(dest)
	if err := m.cacheDigest(dest, digest); err != nil {
		return err
	}
	if !options.SkipHooks {
		if err := runCommands(m.Spec.Build.After); err != nil {
			return fmt.Errorf("post-build hook failure: %v", err)
		}
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
	baseImage, err := m.Spec.Build.GetBaseImage(m.Dir())
	if err != nil {
		return err
	}
	baseImage = strings.Split(baseImage, ":")[0]
	//fmt.Printf("baseImage=%s, builtImages=%#v\n", baseImage, m.BuiltImages)
	newOptions := *options
	for _, builtImage := range m.BuiltImages {
		if baseImage == strings.Split(builtImage, ":")[0] {
			newOptions.Force = true
		}
	}
	err, _ = m.pool.Process(&buildJob{
		m:       m,
		options: &newOptions,
	}).(error)
	if err != nil {
		return err
	}
	return nil
}

func runCommands(commands []Command) error {
	for _, command := range commands {
		cmd := exec.Command(command.Name, command.Args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
