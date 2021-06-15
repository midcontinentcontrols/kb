package kindest

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/labels"

	corev1types "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/midcontinentcontrols/kindest/pkg/kubeconfig"
	"helm.sh/helm/v3/pkg/chart"

	"k8s.io/apimachinery/pkg/api/errors"
	kinderrors "sigs.k8s.io/kind/pkg/errors"
	kindexec "sigs.k8s.io/kind/pkg/exec"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/fs"

	"github.com/Jeffail/tunny"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
)

type TestOptions struct {
	BuildOptions

	KubeContext string `json:"kubeContext,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Transient   bool   `json:"transient,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	SkipBuild   bool   `json:"skipBuild,omitempty"`
}

var ErrMultipleTestEnv = fmt.Errorf("multiple test environments defined")

var ErrNoTestEnv = fmt.Errorf("no test environment")

func (t *TestSpec) Verify(manifestPath string, log logger.Logger) error {
	if err := t.Build.Verify(manifestPath, log); err != nil {
		return err
	}
	if t.Env.Docker != nil {
		if t.Env.Kubernetes != nil {
			return ErrMultipleTestEnv
		}
		return t.Env.Docker.Verify(manifestPath)
	} else if t.Env.Kubernetes != nil {
		return t.Env.Kubernetes.Verify(manifestPath, log)
	} else {
		return ErrNoTestEnv
	}
}

func (t *TestSpec) Run(
	options *TestOptions,
	manifestPath string,
	p *Process,
	log logger.Logger,
) error {
	rootDir := filepath.Dir(manifestPath)
	m := p.GetModuleFromTestSpec(manifestPath+":"+t.Name, t)
	if !options.SkipBuild {
		if err := m.Build(&options.BuildOptions); err != nil {
			return fmt.Errorf("pretest build: %v", err)
		}
	}
	if t.Env.Docker != nil {
		return t.runDocker(rootDir, log)
	} else if t.Env.Kubernetes != nil {
		kubeContext := options.KubeContext
		if options.Kind != "" {
			// Ensure cluster is created
			var err error
			kubeContext, err = cluster_management.CreateCluster(options.Kind, log)
			if err != nil {
				return err
			}
			if options.Transient {
				defer func() {
					log.Info("Deleting transient cluster")
					if err := cluster_management.DeleteCluster(
						options.Kind,
					); err != nil {
						log.Error("Error deleting cluster", zap.Error(err))
					}
				}()
			}
		}
		if err := m.Deploy(&DeployOptions{
			KubeContext: kubeContext,
		}); err != nil {
			return fmt.Errorf("deploy: %v", err)
		}
		return t.runKubernetes(
			kubeContext,
			options.Repository,
			options.Namespace,
			log,
		)
	} else {
		panic("unreachable branch detected")
	}
}

func buildConfigFromFlags(context, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func clientForContext(context string) (*kubernetes.Clientset, *restclient.Config, error) {
	// TODO: in-cluster config
	kubeConfigPath := filepath.Join(homeDir(), ".kube", "config")
	config, err := buildConfigFromFlags(context, kubeConfigPath)
	if err != nil {
		return nil, nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return client, config, nil
}

func clientForKindCluster(name string, provider *cluster.Provider) (*kubernetes.Clientset, string, error) {
	if err := kubeconfig.Save(provider, name, "", false); err != nil {
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
	return client, "kind-" + name, nil
}

func createTestRole(client *kubernetes.Clientset, log logger.Logger) error {
	rbac := client.RbacV1()
	clusterroles := rbac.ClusterRoles()
	if _, err := clusterroles.Get(
		context.TODO(),
		"kindest-test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test role")
			if _, err := client.RbacV1().ClusterRoles().Create(
				context.TODO(),
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kindest-test",
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

func createTestRoleBinding(client *kubernetes.Clientset, log logger.Logger) error {
	if _, err := client.RbacV1().ClusterRoleBindings().Get(
		context.TODO(),
		"kindest-test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test role binding")
			if _, err := client.RbacV1().ClusterRoleBindings().Create(
				context.TODO(),
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kindest-test",
					},
					RoleRef: rbacv1.RoleRef{
						Name:     "kindest-test",
						Kind:     "ClusterRole",
						APIGroup: "rbac.authorization.k8s.io",
					},
					Subjects: []rbacv1.Subject{{
						Kind:      "ServiceAccount",
						Name:      "kindest-test",
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

func createTestServiceAccount(client *kubernetes.Clientset, log logger.Logger) error {
	if _, err := client.CoreV1().ServiceAccounts("default").Get(
		context.TODO(),
		"kindest-test",
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Creating test service account")
			if _, err := client.CoreV1().ServiceAccounts("default").Create(
				context.TODO(),
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kindest-test",
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

func createTestRBAC(client *kubernetes.Clientset, log logger.Logger) error {
	if err := createTestRole(client, log); err != nil {
		return err
	}
	if err := createTestRoleBinding(client, log); err != nil {
		return err
	}
	if err := createTestServiceAccount(client, log); err != nil {
		return err
	}
	return nil
}

func applyManifests(
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

func getSpecImages(spec *KindestSpec, rootPath string) ([]string, error) {
	var images []string
	for i, dependency := range spec.Dependencies {
		if err := func() error {
			depPath := filepath.Clean(filepath.Join(rootPath, dependency))
			otherSpec := &KindestSpec{}
			docBytes, err := ioutil.ReadFile(filepath.Join(depPath, "kindest.yaml"))
			if err != nil {
				return err
			}
			if err := yaml.Unmarshal(docBytes, otherSpec); err != nil {
				return err
			}
			others, err := getSpecImages(otherSpec, depPath)
			if err != nil {
				return err
			}
			images = append(images, others...)
			return nil
		}(); err != nil {
			return nil, fmt.Errorf("dependency.%d (%s): %v", i, dependency, err)
		}
	}
	if spec.Build != nil {
		images = append(images, spec.Build.Name+":latest")
	}
	return images, nil
}

func loadImagesOnCluster(
	imageNames []string,
	name string,
	provider *cluster.Provider,
	concurrency int,
	log logger.Logger,
) error {
	if concurrency == 0 {
		concurrency = runtime.NumCPU()
	}
	log = log.With(zap.String("cluster", name))
	log.Info("Copying images onto cluster",
		zap.Int("numImages", len(imageNames)),
		zap.Int("concurrency", concurrency),
		zap.String("imageNames", fmt.Sprintf("%#v", imageNames)))
	pool := tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		imageName := payload.(string)
		log := log.With(zap.String("image", imageName))
		log.Info("Copying image onto cluster")
		start := time.Now()
		stop := make(chan int, 1)
		defer func() {
			stop <- 0
			close(stop)
		}()
		go func() {
			for {
				select {
				case <-time.After(time.Second):
					log.Info("Still copying image onto cluster", zap.String("elapsed", time.Now().Sub(start).String()))
				case <-stop:
					return
				}
			}
		}()
		if err := loadImageOnCluster(imageName, name, provider); err != nil {
			return err
		}
		log.Info("Copied image onto cluster", zap.String("elapsed", time.Now().Sub(start).String()))
		return nil
	})
	defer pool.Close()
	numImages := len(imageNames)
	dones := make([]chan error, numImages, numImages)
	for i, imageName := range imageNames {
		done := make(chan error, 1)
		dones[i] = done
		go func(imageName string, done chan<- error) {
			defer close(done)
			done <- func() error {
				err, _ := pool.Process(imageName).(error)
				return err
			}()
		}(imageName, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("%s: %v", imageNames[i], err))
		}
	}
	return multi
}

func loadImageOnCluster(imageName, name string, provider *cluster.Provider) error {
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

func waitForCluster(client *kubernetes.Clientset, log logger.Logger) error {
	timeout := time.Second * 120
	delay := time.Second
	start := time.Now()
	good := false
	deadline := time.Now().Add(timeout)
	p := client.CoreV1().Pods("kube-system")
	for time.Now().Before(deadline) {
		pods, err := p.List(context.TODO(), metav1.ListOptions{})
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
	good = false
	sa := client.CoreV1().ServiceAccounts("default")
	for time.Now().Before(deadline) {
		if _, err := sa.Get(
			context.TODO(),
			"default",
			metav1.GetOptions{},
		); err != nil {
			if errors.IsNotFound(err) {
				log.Info("Waiting on default serviceaccount",
					zap.String("elapsed", time.Now().Sub(start).String()),
					zap.String("timeout", timeout.String()))
				time.Sleep(delay)
				continue
			}
			return err
		}
		good = true
		break
	}
	if !good {
		return fmt.Errorf("default serviceaccount failed to appear within %v", timeout.String())
	}
	log.Info("Cluster is running", zap.String("elapsed", time.Now().Sub(start).String()))
	return nil
}

// isChartInstallable validates if a chart can be installed
//
// Application chart type is only installable
func isChartInstallable(ch *chart.Chart) (bool, error) {
	switch ch.Metadata.Type {
	case "", "application":
		return true, nil
	}
	return false, fmt.Errorf("%s charts are not installable", ch.Metadata.Type)
}

var ErrUnknownCluster = fmt.Errorf("unknown cluster")

func ensureClusterExists(name string) error {
	return nil
}

func generateKindConfig(regName string, regPort int) string {
	return fmt.Sprintf(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:%d"]
    endpoint = ["http://%s:%d"]`, regPort, regName, regPort)
}

//  extraMounts:
//  - containerPath: /var/lib/etcd
//    hostPath: /tmp/etcd`

func deleteOldPods(pods corev1types.PodInterface, envName string, log logger.Logger) error {
	stop := make(chan int)
	defer func() {
		stop <- 0
		close(stop)
	}()
	go func() {
		start := time.Now()
		for {
			select {
			case <-time.After(5 * time.Second):
				log.Info("Still deleting old test pods", zap.String("elapsed", time.Now().Sub(start).String()))
			case <-stop:
				return
			}
		}
	}()
	set := labels.Set(map[string]string{
		"kindest": envName,
	})
	listOptions := metav1.ListOptions{LabelSelector: set.AsSelector().String()}
	oldPods, err := pods.List(
		context.TODO(),
		listOptions,
	)
	if err != nil {
		return err
	}
	for _, pod := range oldPods.Items {
		if err := pods.Delete(
			context.TODO(),
			pod.Name,
			metav1.DeleteOptions{},
		); err != nil {
			return err
		}
	}
	return nil
}

/*
func restartDeployments(client *kubernetes.Clientset, restartImages []string, log logger.Logger) error {
	deployments, err := client.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	var waitFor []int
	for i, deployment := range deployments.Items {
		found := false
		for _, image := range restartImages {
			if deployment.Spec.Template.Spec.RestartPolicy == corev1.RestartPolicyAlways {
				for _, container := range deployment.Spec.Template.Spec.Containers {
					if container.Image == image {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		if found {
			waitFor = append(waitFor, i)
			log := log.With(
				zap.String("deployment.Name", deployment.Name),
				zap.String("deployment.Namespace", deployment.Namespace),
			)
			log.Debug("Restarting deployment")
			if deployment.Spec.Template.ObjectMeta.Annotations != nil {
				deployment.Spec.Template.ObjectMeta.Annotations["kindest.io/restartedAt"] = time.Now().String()
			} else {
				deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{
					"kindest.io/restartedAt": time.Now().String(),
				}
			}
			if _, err := client.AppsV1().Deployments(deployment.Namespace).Update(
				context.TODO(),
				&deployment,
				metav1.UpdateOptions{},
			); err != nil {
				log.Error("Error restarting deployment", zap.String("err", err.Error()))
			}
		}
	}
	done := make(chan int)
	go func() {
		for {
			select {
			case <-time.After(5 * time.Second):
				log.Info("Waiting for deployments")
			case <-done:
				return
			}
		}
	}()
	defer func() {
		done <- 0
		close(done)
	}()
	//n := len(waitFor)
	//dones := make([]chan error, n, n)
	timeout := 120 * time.Second
	delay := 3 * time.Second
	deadline := time.Now().Add(timeout)
	for _, j := range waitFor {
		deployment := &deployments.Items[j]
		name := deployment.Name
		namespace := deployment.Namespace
		//done := make(chan error, 1)
		//dones[i] = done
		//go func(name string, namespace string, done chan<- error) {
		log := log.With(
			zap.String("deployment.Name", name),
			zap.String("deployment.Namespace", namespace),
		)
		//defer close(done)
		//done <- func() error {
		getter := client.AppsV1().Deployments(namespace)
		ready := false
		for time.Now().Before(deadline) {
			deployment, err := getter.Get(
				context.TODO(),
				name,
				metav1.GetOptions{},
			)
			if err != nil {
				return err
			}
			var replicas int32 = 1
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			if deployment.Status.AvailableReplicas >= replicas {
				log.Debug("Deployment is ready", zap.Int32("replicas", replicas))
				ready = true
				break
			}
			time.Sleep(delay)
		}
		if !ready {
			fmt.Errorf("%s.%s: %v", deployment.Name, deployment.Namespace, err)
		}
		//return fmt.Errorf("failed to be ready within %v", timeout)
		//}()
		//}(deployment.Name, deployment.Namespace, done)
	}
	return nil
	//var multi error
	//for i, j := range waitFor {
	//	if err := <-dones[i]; err != nil {
	//		deployment := &deployments.Items[j]
	//		multi = multierror.Append(multi, fmt.Errorf("%s.%s: %v", deployment.Name, deployment.Namespace, err))
	//	}
	//}
	//return multi
}
*/

func restartPods(client *kubernetes.Clientset, restartImages []string, log logger.Logger) error {
	pods, err := client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		found := false
		for _, image := range restartImages {
			if pod.Spec.RestartPolicy == corev1.RestartPolicyAlways {
				for _, container := range pod.Spec.Containers {
					if container.Image == image {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		if found {
			log := log.With(
				zap.String("pod.Name", pod.Name),
				zap.String("pod.Namespace", pod.Namespace),
			)
			log.Debug("Restarting pod")
			if err := client.CoreV1().Pods(pod.Namespace).Delete(
				context.TODO(),
				pod.Name,
				metav1.DeleteOptions{},
			); err != nil {
				log.Error("Error restarting pod", zap.String("err", err.Error()))
			}
		}
	}
	return nil
}

/*
func (t *TestSpec) runKubernetes(
	rootPath string,
	options *TestOptions,
	spec *KindestSpec,
	restartImages []string,
	log logger.Logger,
) error {
	isKind := options.Kind != "" || options.Transient
	var client *kubernetes.Clientset
	var kubeContext string
	image := sanitizeImageName(options.Repository, t.Build.Name, "latest")
	imagePullPolicy := corev1.PullAlways
	if isKind {
		cli, err := dockerclient.NewEnvClient()
		if err != nil {
			return err
		}
		name := options.Kind
		if name == "" {
			name = "test-" + uuid.New().String()[:8]
		}
		provider := cluster.NewProvider()
		exists := false
		if !options.Transient {
			clusters, err := provider.List()
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				if cluster == options.Kind {
					exists = true
					break
				}
			}
		}
		if exists {
			log.Info("Using existing kind cluster", zap.String("name", options.Kind))
		} else {
			log := log.With(
				zap.String("name", name),
				zap.Bool("transient", options.Transient),
			)
			log.Info("Creating cluster")
			ready := make(chan int, 1)
			go func() {
				start := time.Now()
				for {
					select {
					case <-time.After(5 * time.Second):
						log.Info("Still creating cluster", zap.String("elapsed", time.Now().Sub(start).String()))
					case <-ready:
						return
					}
				}
			}()
			kindConfig := generateKindConfig("kind-registry", 5000)
			err := provider.Create(name, cluster.CreateWithRawConfig([]byte(kindConfig)))
			ready <- 0
			if err != nil {
				return fmt.Errorf("create cluster: %v", err)
			}
			if options.Transient {
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
			}
		}
		client, kubeContext, err = clientForKindCluster(name, provider)
		if err != nil {
			return err
		}
		if err := waitForCluster(client, log); err != nil {
			return err
		}
		if !options.SkipBuild {
			if options.NoRegistry {
				imagePullPolicy = corev1.PullNever
				images, err := getSpecImages(spec, rootPath)
				if err != nil {
					return err
				}
				images = append(images, image)
				if err := loadImagesOnCluster(
					images,
					name,
					provider,
					options.Concurrency,
					log,
				); err != nil {
					return err
				}
			} else {
				if err := registry.EnsureLocalRegistryRunning(cli, log); err != nil {
					return err
				}
				log := log.With(zap.String("image", image))
				log.Info("Pushing image to local registry")
				if err := cli.NetworkConnect(
					context.TODO(),
					"kind",
					"kind-registry",
					&networktypes.EndpointSettings{},
				); err != nil && !strings.Contains(err.Error(), "Error response from daemon: endpoint with name kind-registry already exists in network kind") {
					return err
				}
				resp, err := cli.ImagePush(
					context.TODO(),
					image,
					types.ImagePushOptions{
						RegistryAuth: "this_can_be_anything",
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
					return fmt.Errorf("push: %v", err)
				}
				log.Info("Pushed image")
			}
		}
	} else if options.Context != "" {
		// Use existing kubernetes context from ~/.kube/config
		var err error
		kubeContext = options.Context
		client, _, err = clientForContext(options.Context)
		if err != nil {
			return err
		}
		// TODO: push image to registry
	} else {
		// We didn't specify an existing cluster and we didn't
		// request a transient cluster. It's unclear where the
		// user is expecting these tests to run.
		return ErrUnknownCluster
	}

	start := time.Now()

	log.Debug("Checking RBAC...")
	if err := createTestRBAC(client, log); err != nil {
		return err
	}

	if err := applyManifests(
		kubeContext,
		rootPath,
		t.Env.Kubernetes.Resources,
	); err != nil {
		return err
	}
	if err := t.installCharts(
		rootPath,
		kubeContext,
		options,
		log,
	); err != nil {
		return err
	}

	//log.Info("Restarting deployments", zap.String("restartImages", fmt.Sprintf("%#v", restartImages)))
	//if err := restartDeployments(client, restartImages); err != nil {
	//	return err
	//}

	namespace := "default"
	pods := client.CoreV1().Pods(namespace)
	log = log.With(
		zap.String("t.Name", t.Name),
		zap.String("namespace", namespace),
		zap.String("image", image),
	)

	if err := deleteOldPods(pods, t.Name, log); err != nil {
		return err
	}

	// Wait for the rest of the the cluster to be Ready
	if err := waitForFullReady(client, log); err != nil {
		return err
	}

	log.Debug("Creating test pod")

	podName := t.Name + "-" + uuid.New().String()[:8]
	var env []corev1.EnvVar
	for _, v := range t.Variables {
		env = append(env, corev1.EnvVar{
			Name:  v.Name,
			Value: v.Value,
		})
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"kindest": t.Name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "kindest-test",
			RestartPolicy:      corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            t.Name,
				Image:           image,
				ImagePullPolicy: imagePullPolicy,
				Command:         t.Build.Command,
				Env:             env,
			}},
		},
	}
	var err error
	if pod, err = pods.Create(
		context.TODO(),
		pod,
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	log.Debug("Created pod")
	if !options.Transient {
		defer func() {
			log.Warn("TODO: clean up pod")
		}()
	}
	timeout := 90 * time.Second
	delay := time.Second
	start = time.Now()
	scheduled := false
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		pod, err = pods.Get(
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
				if status.State.Terminated != nil {
					if code := status.State.Terminated.ExitCode; code != 0 {
						return fmt.Errorf("pod failed with exit code '%d'", code)
					}
					return nil
				}
				if status.State.Waiting != nil {
					if strings.Contains(status.State.Waiting.Reason, "Err") {
						return fmt.Errorf("pod failed with '%s'", status.State.Waiting.Reason)
					}
				}
			}
			log.Info("Still waiting on pod",
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
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Terminated != nil {
					if strings.Contains(status.State.Terminated.Reason, "Err") {
						return ErrTestFailed
					}
				}
			}
			req := pods.GetLogs(pod.Name, &corev1.PodLogOptions{
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
				//log.Info("Test output", zap.String("message", message))
				//fmt.Println(message)
			}
		default:
			return fmt.Errorf("unexpected phase '%s'", pod.Status.Phase)
		}
		if pod, err = pods.Get(
			context.TODO(),
			pod.Name,
			metav1.GetOptions{},
		); err != nil {
			return err
		}
		if pod.Status.Phase == corev1.PodRunning {
			time.Sleep(delay)
			log.Warn("Log stream terminated prematurely. Retailing logs...")
			continue
		} else if pod.Status.Phase == corev1.PodSucceeded {
			return nil
		} else if pod.Status.Phase == corev1.PodFailed {
			// This should NOT happen. Container terminated status
			// should exist if the phase is Failed.
			return ErrTestFailed
		} else {
			return fmt.Errorf("unexpected pod phase '%s'", pod.Status.Phase)
		}
	}
	return ErrPodTimeout
}
*/
var ErrNoTests = fmt.Errorf("no tests configured")

type testRun struct {
	test         *TestSpec
	options      *TestOptions
	spec         *KindestSpec
	manifestPath string
}
