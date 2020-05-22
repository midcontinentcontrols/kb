package kindest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	kinderrors "sigs.k8s.io/kind/pkg/errors"
	kindexec "sigs.k8s.io/kind/pkg/exec"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/google/uuid"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/fs"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/client"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
)

type TestOptions struct {
	File        string `json:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	Transient   bool   `json:"transient,omitempty"`
}

type TestSpec struct {
	Name  string    `json:"name"`
	Build BuildSpec `json:"build"`
	Env   EnvSpec   `json:"env,omitempty"`
}

var ErrMultipleTestEnv = fmt.Errorf("multiple test environments defined")
var ErrNoTestEnv = fmt.Errorf("no test environment")

func (t *TestSpec) Verify(manifestPath string) error {
	if err := t.Build.Verify(manifestPath); err != nil {
		return err
	}
	if t.Env.Docker != nil {
		if t.Env.Kind != nil {
			return ErrMultipleTestEnv
		}
		return nil
	} else if kind := t.Env.Kind; kind != nil {
		rootDir := filepath.Dir(manifestPath)
		for _, resource := range kind.Resources {
			resourcePath := filepath.Clean(filepath.Join(rootDir, resource))
			if _, err := os.Stat(resourcePath); err != nil {
				return fmt.Errorf("test '%s' env: '%s' not found", t.Name, resourcePath)
			}
		}
		for _, chart := range kind.Charts {
			chartPath := filepath.Join(chart.Path, "Chart.yaml")
			if _, err := os.Stat(chartPath); err != nil {
				return fmt.Errorf("test '%s' env chart '%s': missing Chart.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
			}
			valuesPath := filepath.Join(chart.Path, "values.yaml")
			if _, err := os.Stat(valuesPath); err != nil {
				return fmt.Errorf("test '%s' env chart '%s': missing values.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
			}
		}
		return nil
	} else {
		return ErrNoTestEnv
	}
}

func (t *TestSpec) runDocker(
	options *TestOptions,
	cli client.APIClient,
) error {
	var env []string
	for _, v := range t.Env.Variables {
		env = append(env, fmt.Sprintf("%s=%s", v.Name, v.Value))
	}
	resp, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: t.Build.Name + ":latest",
			Env:   env,
		},
		&containertypes.HostConfig{
			AutoRemove: true,
		},
		nil,
		"",
	)
	if err != nil {
		return err
	}
	container := resp.ID
	log := log.With(zap.String("test.Name", t.Name))
	for _, warning := range resp.Warnings {
		log.Debug("Docker", zap.String("warning", warning))
	}
	if err := cli.ContainerStart(
		context.TODO(),
		container,
		types.ContainerStartOptions{},
	); err != nil {
		return err
	}
	logs, err := cli.ContainerLogs(
		context.TODO(),
		container,
		types.ContainerLogsOptions{
			Follow:     true,
			ShowStdout: true,
			ShowStderr: true,
		},
	)
	if err != nil {
		return err
	}
	rd := bufio.NewReader(logs)
	for {
		message, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		log.Info(message)
	}
	ch, e := cli.ContainerWait(
		context.TODO(),
		container,
		containertypes.WaitConditionRemoved,
	)
	select {
	case v := <-ch:
		if v.Error != nil {
			return fmt.Errorf(v.Error.Message)
		}
		if v.StatusCode != 0 {
			return fmt.Errorf("exit code %d", v.StatusCode)
		}
		return nil
	case err := <-e:
		return err
	}
}

func clientForCluster(name string, provider *cluster.Provider) (*kubernetes.Clientset, string, error) {
	if err := provider.ExportKubeConfig(name, ""); err != nil {
		return nil, "", err
	}
	kubeConfig, err := provider.KubeConfig(name, false)
	if err != nil {
		return nil, "", err
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, "", err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", err
	}
	return client, "", nil
}

func createTestRole(client *kubernetes.Clientset) error {
	if _, err := client.RbacV1().ClusterRoles().Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test role")
			if _, err := client.RbacV1().ClusterRoles().Create(
				context.TODO(),
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Rules: []rbacv1.PolicyRule{{
						APIGroups: []string{"*"},
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					}},
				},
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Debug("Created test role")
		} else {
			return err
		}
	} else {
		log.Debug("Test role already created")
	}
	return nil
}

func createTestRoleBinding(client *kubernetes.Clientset) error {
	if _, err := client.RbacV1().ClusterRoleBindings().Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test role binding")
			if _, err := client.RbacV1().ClusterRoleBindings().Create(
				context.TODO(),
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					RoleRef: rbacv1.RoleRef{
						Name:     "test",
						Kind:     "ClusterRole",
						APIGroup: "rbac.authorization.k8s.io",
					},
					Subjects: []rbacv1.Subject{{
						Kind:      "ServiceAccount",
						Name:      "test",
						Namespace: "default",
					}},
				},
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Debug("Created test role binding")
		} else {
			return err
		}
	} else {
		log.Debug("Test role binding already created")
	}
	return nil
}
func createTestServiceAccount(client *kubernetes.Clientset) error {
	if _, err := client.CoreV1().ServiceAccounts("default").Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test service account")
			if _, err := client.CoreV1().ServiceAccounts("default").Create(
				context.TODO(),
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
				},
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Debug("Created test service account")
		} else {
			return err
		}
	} else {
		log.Debug("Test service account already created")
	}
	return nil
}

func createTestRBAC(client *kubernetes.Clientset) error {
	if err := createTestRole(client); err != nil {
		return err
	}
	if err := createTestRoleBinding(client); err != nil {
		return err
	}
	if err := createTestServiceAccount(client); err != nil {
		return err
	}
	return nil
}

func applyTestManifests(
	kubeContext string,
	rootPath string,
	resources []string,
) error {
	for _, resource := range resources {
		resource = filepath.Clean(filepath.Join(rootPath, resource))
		cmd := exec.Command("kubectl", "apply", "--context", kubeContext, "-f", resource)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

var ErrTestFailed = fmt.Errorf("test failed")

var ErrPodTimeout = fmt.Errorf("pod timed out")

// save saves image to dest, as in `docker save`
func save(image, dest string) error {
	return exec.Command("docker", "save", "-o", dest, image).Run()
}

// loads an image tarball onto a node
func loadImage(imageTarName string, node nodes.Node) error {
	f, err := os.Open(imageTarName)
	if err != nil {
		return fmt.Errorf("failed to open image: %v", err)
	}
	defer f.Close()
	return nodeutils.LoadImageArchive(node, f)
}

// imageID return the Id of the container image
func imageID(containerNameOrID string) (string, error) {
	cmd := kindexec.Command("docker", "image", "inspect",
		"-f", "{{ .Id }}",
		containerNameOrID, // ... against the container
	)
	lines, err := kindexec.CombinedOutputLines(cmd)
	if err != nil {
		return "", err
	}
	if len(lines) != 1 {
		return "", fmt.Errorf("Docker image ID should only be one line, got %d lines", len(lines))
	}
	return lines[0], nil
}

func loadImageOnCluster(name string, imageName string, provider *cluster.Provider) error {
	imageID, err := imageID(imageName)
	if err != nil {
		return fmt.Errorf("image: %q not present locally", imageName)
	}

	nodeList, err := provider.ListInternalNodes(name)
	if err != nil {
		return err
	}
	if len(nodeList) == 0 {
		return fmt.Errorf("no nodes found for cluster %q", name)
	}

	// map cluster nodes by their name
	nodesByName := map[string]nodes.Node{}
	for _, node := range nodeList {
		// TODO(bentheelder): this depends on the fact that ListByCluster()
		// will have name for nameOrId.
		nodesByName[node.String()] = node
	}
	candidateNodes := nodeList
	// pick only the nodes that don't have the image
	selectedNodes := []nodes.Node{}
	for _, node := range candidateNodes {
		id, err := nodeutils.ImageID(node, imageName)
		if err != nil || id != imageID {
			selectedNodes = append(selectedNodes, node)
			//logger.V(0).Infof("Image: %q with ID %q not yet present on node %q, loading...", imageName, imageID, node.String())
		}
	}
	if len(selectedNodes) == 0 {
		return nil
	}
	dir, err := fs.TempDir("", "image-tar")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %v", err)
	}
	defer os.RemoveAll(dir)
	imageTarPath := filepath.Join(dir, "image.tar")

	err = save(imageName, imageTarPath)
	if err != nil {
		return err
	}
	// Load the image on the selected nodes
	fns := []func() error{}
	for _, selectedNode := range selectedNodes {
		selectedNode := selectedNode // capture loop variable
		fns = append(fns, func() error {
			return loadImage(imageTarPath, selectedNode)
		})
	}
	return kinderrors.UntilErrorConcurrent(fns)
}

func waitForCluster(name string, client *kubernetes.Clientset) error {
	log := log.With(zap.String("name", name))
	timeout := time.Second * 120
	delay := time.Second
	start := time.Now()
	good := false
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		pods, err := client.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		count := 0
		for _, pod := range pods.Items {
			switch pod.Status.Phase {
			case corev1.PodPending:
				count++
			case corev1.PodRunning:
				continue
			default:
				return fmt.Errorf("unexpected pod phase '%s' for %s.%s", pod.Status.Phase, pod.Name, pod.Namespace)
			}
		}
		if count == 0 {
			good = true
			break
		}
		total := len(pods.Items)
		log.Info("Waiting on pods in kube-system",
			zap.Int("numReady", total-count),
			zap.Int("numPods", total),
			zap.String("elapsed", time.Now().Sub(start).String()),
			zap.String("timeout", timeout.String()))
		time.Sleep(delay)
	}
	if !good {
		return fmt.Errorf("pods in kube-system failed to be Ready within %s", timeout.String())
	}
	log.Info("Cluster is running", zap.String("elapsed", time.Now().Sub(start).String()))
	return nil
}

func (t *TestSpec) runKind(
	rootPath string,
	options *TestOptions,
	cli client.APIClient,
) error {
	name := "test-" + uuid.New().String()[:8]
	log := log.With(zap.String("name", name))
	provider := cluster.NewProvider()
	if options.Transient {
		log.Info("Creating cluster", zap.Bool("transient", options.Transient))
		if err := provider.Create(name); err != nil {
			return err
		}
		defer func() {
			log.Info("Deleting transient cluster")
			if err := func() error {
				if err := provider.Delete(
					name,
					"",
				); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				log.Error("Error cleaning up transient cluster", zap.String("message", err.Error()))
			} else {
				log.Info("Deleted transient cluster")
			}
		}()
	} else {
		return fmt.Errorf("unimplemented")
	}

	client, kubeContext, err := clientForCluster(name, provider)
	if err != nil {
		return err
	}

	if err := waitForCluster(name, client); err != nil {
		return err
	}

	start := time.Now()
	image := t.Build.Name + ":latest"
	imageLog := log.With(zap.String("image", image))
	imageLog.Info("Loading image onto cluster")
	if err := loadImageOnCluster(
		name,
		image,
		provider,
	); err != nil {
		return err
	}
	imageLog.Info("Loaded image onto cluster", zap.String("elapsed", time.Now().Sub(start).String()))

	log.Debug("Checking RBAC...")
	if err := createTestRBAC(client); err != nil {
		return err
	}

	if err := applyTestManifests(
		kubeContext,
		rootPath,
		t.Env.Kind.Resources,
	); err != nil {
		return err
	}

	// TODO: install/upgrade helm charts

	namespace := "default"
	pods := client.CoreV1().Pods(namespace)
	podName := "test-" + t.Name
	podLog := log.With(zap.String("name", podName), zap.String("namespace", namespace))
	podLog.Debug("Creating pod")
	if _, err := pods.Create(
		context.TODO(),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: "test",
				Containers: []corev1.Container{{
					Name:            t.Name,
					Image:           t.Build.Name + ":latest",
					ImagePullPolicy: corev1.PullNever,
				}},
			},
		},
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	podLog.Debug("Created pod")
	if !options.Transient {
		defer func() {
			podLog.Warn("TODO: clean up pod")
		}()
	}
	timeout := 90 * time.Second
	delay := time.Second
	start = time.Now()
	scheduled := false
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		pod, err := pods.Get(
			context.TODO(),
			podName,
			metav1.GetOptions{},
		)
		if err != nil {
			return err
		}
		switch pod.Status.Phase {
		case corev1.PodPending:
			if !scheduled {
				for _, condition := range pod.Status.Conditions {
					if condition.Status == "PodScheduled" {
						deadline = time.Now().Add(30 * time.Second)
						scheduled = true
					}
				}
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Waiting != nil {
					if strings.Contains(status.State.Waiting.Reason, "Err") {
						return fmt.Errorf("pod failed with '%s'", status.State.Waiting.Reason)
					}
				}
			}
			podLog.Info("Still waiting on pod",
				zap.String("phase", string(pod.Status.Phase)),
				zap.Bool("scheduled", scheduled),
				zap.String("elapsed", time.Now().Sub(start).String()),
				zap.String("timeout", timeout.String()))
			time.Sleep(delay)
			continue
		case corev1.PodRunning:
			fallthrough
		case corev1.PodSucceeded:
			fallthrough
		case corev1.PodFailed:
			req := pods.GetLogs(podName, &corev1.PodLogOptions{
				Follow: true,
			})
			r, err := req.Stream(context.TODO())
			if err != nil {
				return err
			}
			rd := bufio.NewReader(r)
			for {
				message, err := rd.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				fmt.Println(message)
			}
		default:
			return fmt.Errorf("unexpected phase '%s'", pod.Status.Phase)
		}
		if pod.Status.Phase == corev1.PodSucceeded {
			return nil
		} else if pod.Status.Phase == corev1.PodFailed {
			return ErrTestFailed
		} else {
			panic("unreachable branch detected")
		}
	}
	return ErrPodTimeout
}

func (t *TestSpec) Run(
	manifestPath string,
	options *TestOptions,
	cli client.APIClient,
) error {
	if err := t.Build.Build(
		manifestPath,
		&BuildOptions{
			Concurrency: 1,
		},
		cli,
		nil,
	); err != nil {
		return err
	}
	if t.Env.Docker != nil {
		return t.runDocker(options, cli)
	} else if t.Env.Kind != nil {
		return t.runKind(filepath.Dir(manifestPath), options, cli)
	} else {
		panic("unreachable branch detected")
	}
}

var ErrNoTests = fmt.Errorf("no tests configured")

type testRun struct {
	test         *TestSpec
	options      *TestOptions
	manifestPath string
}

func Test(options *TestOptions) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	var pool *tunny.Pool
	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = runtime.NumCPU()
	}
	pool = tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		item := payload.(*testRun)
		return item.test.Run(item.manifestPath, item.options, cli)
	})
	defer pool.Close()
	return TestEx(options, pool)
}

func TestEx(options *TestOptions, pool *tunny.Pool) error {
	spec, manifestPath, err := loadSpec(options.File)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	if len(spec.Test) == 0 {
		return ErrNoTests
	}
	n := len(spec.Test)
	dones := make([]chan error, n, n)
	for i, test := range spec.Test {
		done := make(chan error, 1)
		dones[i] = done
		go func(test *TestSpec, done chan<- error) {
			defer close(done)
			err, _ := pool.Process(&testRun{
				manifestPath: manifestPath,
				test:         test,
				options:      options,
			}).(error)
			done <- err
		}(test, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("%s: %v", spec.Test[i].Name, err))
		}
	}
	return multi
}
