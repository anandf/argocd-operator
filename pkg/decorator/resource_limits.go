package decorator

import (
	"fmt"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourceLimitsDecorator applies resource limits and requests to pod containers
type ResourceLimitsDecorator struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	ArgoCD         *argoproj.ArgoCD
	ComponentName  string
	DefaultLimits  corev1.ResourceList
	DefaultRequests corev1.ResourceList
	logger         logr.Logger
}

// NewResourceLimitsDecorator creates a new ResourceLimitsDecorator
func NewResourceLimitsDecorator(client client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD, componentName string) *ResourceLimitsDecorator {
	return &ResourceLimitsDecorator{
		Client:        client,
		Scheme:        scheme,
		ArgoCD:        cr,
		ComponentName: componentName,
		DefaultLimits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		DefaultRequests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		logger: logs.Log.WithName("ResourceLimitsDecorator"),
	}
}

func (r *ResourceLimitsDecorator) Decorate(obj runtime.Object) error {
	var podspec *corev1.PodSpec
	var objectName string

	switch typed := obj.(type) {
	case *corev1.Pod:
		podspec = &typed.Spec
		objectName = fmt.Sprintf("Pod/%s", typed.Name)
	case *v1.Deployment:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("Deployment/%s", typed.Name)
	case *v1.DaemonSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("DaemonSet/%s", typed.Name)
	case *v1.ReplicaSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("ReplicaSet/%s", typed.Name)
	case *v1.StatefulSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("StatefulSet/%s", typed.Name)
	case *v2.Job:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("Job/%s", typed.Name)
	case *v2.CronJob:
		podspec = &typed.Spec.JobTemplate.Spec.Template.Spec
		objectName = fmt.Sprintf("CronJob/%s", typed.Name)
	default:
		return fmt.Errorf("unsupported object type for resource limits decoration: %T", obj)
	}

	r.logger.Info("Decorating podspec with resource limits", "object", objectName, "component", r.ComponentName)

	// Apply resource limits and requests to all containers
	for i := range podspec.Containers {
		container := &podspec.Containers[i]

		// Only set if not already configured
		if container.Resources.Limits == nil {
			container.Resources.Limits = r.DefaultLimits.DeepCopy()
		}
		if container.Resources.Requests == nil {
			container.Resources.Requests = r.DefaultRequests.DeepCopy()
		}

		r.logger.V(1).Info("Applied resource limits to container",
			"container", container.Name,
			"limits", container.Resources.Limits,
			"requests", container.Resources.Requests)
	}

	// Apply to init containers as well
	for i := range podspec.InitContainers {
		container := &podspec.InitContainers[i]

		if container.Resources.Limits == nil {
			container.Resources.Limits = r.DefaultLimits.DeepCopy()
		}
		if container.Resources.Requests == nil {
			container.Resources.Requests = r.DefaultRequests.DeepCopy()
		}
	}

	return nil
}
