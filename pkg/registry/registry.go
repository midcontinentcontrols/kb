package registry

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/docker/docker/api/types"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

func CreateLocalRegistry(
	regName string,
	regPort int,
	image string,
	cli client.APIClient,
	log logger.Logger,
) error {
	if _, err := cli.ImagePull(
		context.TODO(),
		image,
		types.ImagePullOptions{},
	); err != nil {
		return err
	}
	portStr := fmt.Sprintf("%d/tcp", regPort)
	info, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: image,
		},
		&containertypes.HostConfig{
			RestartPolicy: containertypes.RestartPolicy{
				Name: "always",
			},
			PortBindings: nat.PortMap(map[nat.Port][]nat.PortBinding{
				nat.Port(portStr): {{
					HostIP:   "127.0.0.1",
					HostPort: portStr,
				}},
			}),
		},
		nil,
		regName,
	)
	if err != nil {
		return err
	}
	if err := util.WaitForContainer(regName, cli, log); err != nil {
		return err
	}
	log = log.With(zap.String("id", info.ID))
	log.Info("Created registry container")
	for _, warning := range info.Warnings {
		log.Warn("Container create warning", zap.String("message", warning))
	}
	return nil
}

func DeleteLocalRegistry(cli client.APIClient, log logger.Logger) error {
	name := "kind-registry"
	timeout := 10 * time.Second
	log = log.With(zap.String("name", name))
	log.Info("Stopping registry container")
	if err := cli.ContainerStop(
		context.TODO(),
		name,
		&timeout,
	); err != nil {
		return err
	}
	force := true
	log.Info("Removing registry container", zap.Bool("force", force))
	if err := cli.ContainerRemove(
		context.TODO(),
		name,
		types.ContainerRemoveOptions{
			Force: force,
		},
	); err != nil {
		return err
	}
	return nil
}

// EnsureLocalRegistryRunning ensures a local docker registry is running,
// as per https://kind.sigs.k8s.io/docs/user/local-registry/
func EnsureLocalRegistryRunning(cli client.APIClient, log logger.Logger) error {
	image := "registry:2"
	if err := util.EnsureImagePulled(image, cli, log); err != nil {
		return err
	}
	regName := "kind-registry"
	regPort := 5000
	// TODO: implement pull if image is not present
	if err := util.WaitForContainer(regName, cli, log); err != nil {
		if client.IsErrNotFound(err) {
			return CreateLocalRegistry(regName, regPort, image, cli, log)
		}
		return err
	}
	return nil
}

func registryService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kindest-registry",
			Namespace: "kindest",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       "registry",
				Port:       5000,
				TargetPort: intstr.FromInt(5000),
				Protocol:   corev1.ProtocolTCP,
			}},
			Selector: map[string]string{
				"app": "kindest-registry",
			},
		},
	}
}

func registryDeployment() *appsv1.Deployment {
	labels := map[string]string{
		"app": "kindest-registry",
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kindest-registry",
			Namespace: "kindest",
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{
						Name: "regcred",
					}},
					Containers: []corev1.Container{{
						Name:            "registry",
						Image:           "registry:2",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{{
							Name:  "REGISTRY_STORAGE_DELETE_ENABLED",
							Value: "true",
						}, {
							Name:  "REGISTRY_STORAGE_INMEMORY",
							Value: "1",
						}},
						Resources: corev1.ResourceRequirements{
							//Limits: v1.ResourceList{
							//	"cpu":    resource.MustParse("100m"),
							//	"memory": resource.MustParse("512Mi"),
							//},
						},
						Ports: []corev1.ContainerPort{{
							Name:          "registry",
							ContainerPort: 5000,
						}},
					}},
				},
			},
		},
	}
}

func ensureDeployment(
	desired *appsv1.Deployment,
	cl *kubernetes.Clientset,
	log logger.Logger,
) error {
	log = log.With(
		zap.String("name", desired.Name),
		zap.String("namespace", desired.Namespace),
	)
	deployments := cl.AppsV1().Deployments(desired.Namespace)
	existing, err := deployments.Get(
		context.TODO(),
		desired.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating deployment")
			if existing, err = deployments.Create(
				context.TODO(),
				desired,
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Info("Deployment created")
		} else {
			return err
		}
	} else {
		log.Info("Deployment already exists")
	}
	var replicas int32 = 1
	if existing.Spec.Replicas != nil {
		replicas = *existing.Spec.Replicas
	}
	if existing.Status.AvailableReplicas >= replicas {
		// All replicas are available
		return nil
	}
	retryInterval := time.Second * 3
	time.Sleep(retryInterval)
	return WaitForDeployment(
		cl,
		existing.Name,
		existing.Namespace,
		retryInterval,
		60*time.Second,
	)
}

func ensureService(
	desired *corev1.Service,
	cl *kubernetes.Clientset,
	log logger.Logger,
) error {
	log = log.With(
		zap.String("name", desired.Name),
		zap.String("namespace", desired.Namespace),
	)
	services := cl.CoreV1().Services(desired.Namespace)
	if _, err := services.Get(
		context.TODO(),
		desired.Name,
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating service")
			if _, err := services.Create(
				context.TODO(),
				desired,
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Info("Service created")
			return nil
		}
		return err
	}
	log.Info("Service already exists")
	return nil
}

func ensureNamespace(
	namespace string,
	cl *kubernetes.Clientset,
) error {
	if _, err := cl.CoreV1().Namespaces().Create(
		context.TODO(),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		},
		metav1.CreateOptions{},
	); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func WaitForDeployment(
	cl *kubernetes.Clientset,
	name string,
	namespace string,
	retryInterval time.Duration,
	timeout time.Duration,
) error {
	deployments := cl.AppsV1().Deployments(namespace)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deployment, err := deployments.Get(
			context.TODO(),
			name,
			metav1.GetOptions{},
		)
		if err == nil {
			var replicas int32 = 1
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			if deployment.Status.AvailableReplicas >= replicas {
				// All replicas are available
				return nil
			}
		} else if err != nil && !errors.IsNotFound(err) {
			return err
		}
		<-time.After(retryInterval)
	}
	return nil
}
