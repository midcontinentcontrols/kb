package kindest

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/docker/docker/api/types"
	"github.com/hashicorp/go-multierror"
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

func CreateLocalRegistry(regName string, regPort int, cli client.APIClient) error {
	portStr := fmt.Sprintf("%d/tcp", regPort)
	info, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: "registry:2",
		},
		&containertypes.HostConfig{
			RestartPolicy: containertypes.RestartPolicy{
				Name: "always",
			},
			PortBindings: nat.PortMap(map[nat.Port][]nat.PortBinding{
				nat.Port(portStr): []nat.PortBinding{{
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
	if err := waitForContainer(regName, cli); err != nil {
		return err
	}
	log := log.With(zap.String("id", info.ID))
	log.Info("Created registry container")
	for _, warning := range info.Warnings {
		log.Warn("Container create warning", zap.String("message", warning))
	}
	return nil
}

func DeleteRegistry(cli client.APIClient) error {
	name := "kind-registry"
	timeout := 10 * time.Second
	log := log.With(zap.String("name", name))
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

// EnsureRegistryRunning ensures a local docker registry is running,
// as per https://kind.sigs.k8s.io/docs/user/local-registry/
func EnsureLocalRegistryRunning(cli client.APIClient) error {
	regName := "kind-registry"
	regPort := 5000
	if err := waitForContainer(regName, cli); err != nil {
		if client.IsErrNotFound(err) {
			return CreateLocalRegistry(regName, regPort, cli)
		}
		return err
	}
	return nil
}

func registryService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kindest-registry",
			Namespace: "default",
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
			Namespace: "default",
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
) error {
	log := log.With(
		zap.String("name", desired.Name),
		zap.String("namespace", desired.Namespace),
	)
	deployments := cl.AppsV1().Deployments(desired.Namespace)
	if _, err := deployments.Get(
		context.TODO(),
		desired.Name,
		metav1.GetOptions{},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating deployment")
			if _, err := deployments.Create(
				context.TODO(),
				desired,
				metav1.CreateOptions{},
			); err != nil {
				return err
			}
			log.Info("Deployment created")
			return nil
		}
		return err
	}
	log.Info("Deployment already exists")
	return nil
}

func ensureService(
	desired *corev1.Service,
	cl *kubernetes.Clientset,
) error {
	log := log.With(
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

func EnsureInClusterRegistryRunning(cl *kubernetes.Clientset) error {
	deploymentDone := make(chan error, 1)
	serviceDone := make(chan error, 1)
	go func() {
		deploymentDone <- ensureDeployment(registryDeployment(), cl)
		close(deploymentDone)
	}()
	go func() {
		serviceDone <- ensureService(registryService(), cl)
		close(serviceDone)
	}()
	var multi error
	if err := <-deploymentDone; err != nil {
		multi = multierror.Append(multi, fmt.Errorf("deployment: %v", err))
	}
	if err := <-serviceDone; err != nil {
		multi = multierror.Append(multi, fmt.Errorf("service: %v", err))
	}
	return multi
}
