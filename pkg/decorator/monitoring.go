package decorator

import (
	"fmt"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

// MonitoringDecorator adds Prometheus monitoring annotations and labels
type MonitoringDecorator struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	ArgoCD        *argoproj.ArgoCD
	ComponentName string
	MetricsPort   string
	MetricsPath   string
	logger        logr.Logger
}

// NewMonitoringDecorator creates a new MonitoringDecorator
func NewMonitoringDecorator(client client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD, componentName string) *MonitoringDecorator {
	return &MonitoringDecorator{
		Client:        client,
		Scheme:        scheme,
		ArgoCD:        cr,
		ComponentName: componentName,
		MetricsPort:   "8082", // Default Argo CD metrics port
		MetricsPath:   "/metrics",
		logger:        logs.Log.WithName("MonitoringDecorator"),
	}
}

func (m *MonitoringDecorator) Decorate(obj runtime.Object) error {
	var podTemplateSpec *corev1.PodTemplateSpec
	var objectName string

	switch typed := obj.(type) {
	case *v1.Deployment:
		podTemplateSpec = &typed.Spec.Template
		objectName = fmt.Sprintf("Deployment/%s", typed.Name)
	case *v1.DaemonSet:
		podTemplateSpec = &typed.Spec.Template
		objectName = fmt.Sprintf("DaemonSet/%s", typed.Name)
	case *v1.ReplicaSet:
		podTemplateSpec = &typed.Spec.Template
		objectName = fmt.Sprintf("ReplicaSet/%s", typed.Name)
	case *v1.StatefulSet:
		podTemplateSpec = &typed.Spec.Template
		objectName = fmt.Sprintf("StatefulSet/%s", typed.Name)
	case *v2.Job:
		podTemplateSpec = &typed.Spec.Template
		objectName = fmt.Sprintf("Job/%s", typed.Name)
	case *v2.CronJob:
		podTemplateSpec = &typed.Spec.JobTemplate.Spec.Template
		objectName = fmt.Sprintf("CronJob/%s", typed.Name)
	case *corev1.Pod:
		// For standalone pods, add annotations directly
		m.addMonitoringAnnotations(typed.Annotations)
		m.addMonitoringLabels(typed.Labels)
		return nil
	default:
		return fmt.Errorf("unsupported object type for monitoring decoration: %T", obj)
	}

	m.logger.Info("Decorating object with monitoring annotations", "object", objectName, "component", m.ComponentName)

	// Add Prometheus annotations
	if podTemplateSpec.Annotations == nil {
		podTemplateSpec.Annotations = make(map[string]string)
	}
	m.addMonitoringAnnotations(podTemplateSpec.Annotations)

	// Add monitoring labels
	if podTemplateSpec.Labels == nil {
		podTemplateSpec.Labels = make(map[string]string)
	}
	m.addMonitoringLabels(podTemplateSpec.Labels)

	return nil
}

// addMonitoringAnnotations adds Prometheus scraping annotations
func (m *MonitoringDecorator) addMonitoringAnnotations(annotations map[string]string) {
	annotations["prometheus.io/scrape"] = "true"
	annotations["prometheus.io/port"] = m.MetricsPort
	annotations["prometheus.io/path"] = m.MetricsPath
}

// addMonitoringLabels adds monitoring-related labels
func (m *MonitoringDecorator) addMonitoringLabels(labels map[string]string) {
	labels["app.kubernetes.io/component"] = m.ComponentName
	labels["monitoring"] = "enabled"
}
