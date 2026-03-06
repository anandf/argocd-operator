package server

import (
	"context"
	"embed"
	"fmt"
	"strings"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

const componentName = "server"

var log = logs.Log.WithName("ServerController")

//go:embed templates/*.tmpl
var templateFS embed.FS

// routeGVK is the GroupVersionKind for OpenShift Routes.
var routeGVK = schema.GroupVersionKind{
	Group:   "route.openshift.io",
	Version: "v1",
	Kind:    "Route",
}

// ServerController manages the Argo CD Server component
type ServerController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewServerController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *ServerController {
	return &ServerController{
		Client:         client,
		Scheme:         scheme,
		logger:         log,
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *ServerController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *ServerController) Name() string {
	return "server"
}

func (r *ServerController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return cr.Spec.Server.IsEnabled()
}

func (r *ServerController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server component")

	// Reconcile server service account
	if err := r.reconcileServerServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile server role
	if err := r.reconcileServerRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile server role binding
	if err := r.reconcileServerRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile server cluster role
	if err := r.reconcileServerClusterRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile server cluster role binding
	if err := r.reconcileServerClusterRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile server deployment
	if err := r.reconcileServerDeployment(ctx, cr); err != nil {
		return err
	}

	// Reconcile server service
	if err := r.reconcileServerService(ctx, cr); err != nil {
		return err
	}

	// Reconcile server metrics service
	if err := r.reconcileServerMetricsService(ctx, cr); err != nil {
		return err
	}

	// Reconcile ingress/route based on platform config
	if err := r.reconcileServerIngress(ctx, cr); err != nil {
		return err
	}

	log.Info("server component reconciliation complete")
	return nil
}

func (r *ServerController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("server is not enabled, removing resources")
	name := cr.Name + "-" + componentName

	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	// Metrics service
	if err := component.DeleteResourceIfExists(ctx, r.Client, name+"-metrics", cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.ServiceAccount{}); err != nil {
		return err
	}
	// Namespaced RBAC
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &rbacv1.Role{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &rbacv1.RoleBinding{}); err != nil {
		return err
	}
	// Cluster-scoped RBAC
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, "", &rbacv1.ClusterRole{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, "", &rbacv1.ClusterRoleBinding{}); err != nil {
		return err
	}
	// Clean up ingress/route
	return r.cleanupIngressRoute(ctx, cr)
}

// reconcileServerServiceAccount reconciles the ServiceAccount for the Server
func (r *ServerController) reconcileServerServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server serviceaccount")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("serviceaccount.yaml.tmpl", data)
	if err != nil {
		return err
	}

	sa := &corev1.ServiceAccount{}
	if err := template.ConvertToTyped(obj, sa); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, sa, nil)
}

// reconcileServerRole reconciles the Role for the Server
func (r *ServerController) reconcileServerRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server role")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("role.yaml.tmpl", data)
	if err != nil {
		return err
	}

	role := &rbacv1.Role{}
	if err := template.ConvertToTyped(obj, role); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, role, nil)
}

// reconcileServerRoleBinding reconciles the RoleBinding for the Server
func (r *ServerController) reconcileServerRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server rolebinding")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("rolebinding.yaml.tmpl", data)
	if err != nil {
		return err
	}

	rb := &rbacv1.RoleBinding{}
	if err := template.ConvertToTyped(obj, rb); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, rb, nil)
}

// reconcileServerClusterRole reconciles the ClusterRole for the Server
func (r *ServerController) reconcileServerClusterRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server clusterrole")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("clusterrole.yaml.tmpl", data)
	if err != nil {
		return err
	}

	clusterRole := &rbacv1.ClusterRole{}
	if err := template.ConvertToTyped(obj, clusterRole); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, clusterRole, nil)
}

// reconcileServerClusterRoleBinding reconciles the ClusterRoleBinding for the Server
func (r *ServerController) reconcileServerClusterRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server clusterrolebinding")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("clusterrolebinding.yaml.tmpl", data)
	if err != nil {
		return err
	}

	crb := &rbacv1.ClusterRoleBinding{}
	if err := template.ConvertToTyped(obj, crb); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, crb, nil)
}

// reconcileServerDeployment reconciles the Deployment for the Server
func (r *ServerController) reconcileServerDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server deployment")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-" + componentName).
		WithImage(getServerContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("Command", getServerCommand(cr))

	if cr.Spec.Server.Replicas != nil {
		data.WithExtra("Replicas", *cr.Spec.Server.Replicas)
	}

	resources := getServerResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	if len(cr.Spec.Server.Env) > 0 {
		data.WithExtra("Env", cr.Spec.Server.Env)
	}

	if len(cr.Spec.Server.Volumes) > 0 {
		data.WithExtra("Volumes", cr.Spec.Server.Volumes)
	}
	if len(cr.Spec.Server.VolumeMounts) > 0 {
		data.WithExtra("VolumeMounts", cr.Spec.Server.VolumeMounts)
	}

	component.ApplyNodePlacement(data, cr.Spec.NodePlacement)

	obj, err := r.templateEngine.RenderManifest("deployment.yaml.tmpl", data)
	if err != nil {
		return err
	}

	deploy := &appsv1.Deployment{}
	if err := template.ConvertToTyped(obj, deploy); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, deploy, r.Decorators)
}

// reconcileServerService reconciles the Service for the Server
func (r *ServerController) reconcileServerService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server service")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("ServiceType", getServerServiceType(cr))

	obj, err := r.templateEngine.RenderManifest("service.yaml.tmpl", data)
	if err != nil {
		return err
	}

	svc := &corev1.Service{}
	if err := template.ConvertToTyped(obj, svc); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, svc, nil)
}

// reconcileServerMetricsService reconciles the metrics Service for the Server
func (r *ServerController) reconcileServerMetricsService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server metrics service")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("service-metrics.yaml.tmpl", data)
	if err != nil {
		return err
	}

	svc := &corev1.Service{}
	if err := template.ConvertToTyped(obj, svc); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, svc, nil)
}

// reconcileServerIngress reconciles the Ingress or Route for the Server component
// based on the platform configuration set at construction time
func (r *ServerController) reconcileServerIngress(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	// Check if server ingress/route is enabled in the CR
	if !cr.Spec.Server.Ingress.Enabled && !cr.Spec.Server.Route.Enabled {
		log.Info("server ingress/route not enabled, cleaning up")
		return r.cleanupIngressRoute(ctx, cr)
	}

	// Determine which API to use based on platform config
	if cr.Spec.Server.Route.Enabled && r.config.IsAvailable(component.RouteAPI, r.Client) {
		log.Info("creating server route (OpenShift)")
		return r.reconcileRoute(ctx, cr)
	}

	if cr.Spec.Server.Ingress.Enabled && r.config.IsAvailable(component.IngressAPI, r.Client) {
		log.Info("creating server ingress (Kubernetes)")
		return r.reconcileIngress(ctx, cr)
	}

	if cr.Spec.Server.Route.Enabled || cr.Spec.Server.Ingress.Enabled {
		log.Info("ingress/route enabled but no suitable API configured for platform",
			"routeEnabled", cr.Spec.Server.Route.Enabled,
			"ingressEnabled", cr.Spec.Server.Ingress.Enabled,
			"platformRoute", r.config.IsAvailable(component.RouteAPI, r.Client),
			"platformIngress", r.config.IsAvailable(component.IngressAPI, r.Client))
	}

	return nil
}

// reconcileIngress reconciles the Ingress for the Server
func (r *ServerController) reconcileIngress(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server ingress")

	host := getServerHost(cr)
	path := getPathOrDefault(cr.Spec.Server.Ingress.Path)

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithExtra("Host", host).
		WithExtra("Path", path)

	// Use custom annotations if specified, otherwise defaults are in the template
	if len(cr.Spec.Server.Ingress.Annotations) > 0 {
		data.WithExtra("IngressAnnotations", cr.Spec.Server.Ingress.Annotations)
	}

	// IngressClassName
	if cr.Spec.Server.Ingress.IngressClassName != nil {
		data.WithExtra("IngressClassName", *cr.Spec.Server.Ingress.IngressClassName)
	}

	// TLS configuration
	if len(cr.Spec.Server.Ingress.TLS) > 0 {
		data.WithExtra("TLS", cr.Spec.Server.Ingress.TLS)
	}

	obj, err := r.templateEngine.RenderManifest("ingress.yaml.tmpl", data)
	if err != nil {
		return err
	}

	ingress := &networkingv1.Ingress{}
	if err := template.ConvertToTyped(obj, ingress); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, ingress, nil)
}

// reconcileRoute reconciles the OpenShift Route for the Server.
// Uses unstructured objects to avoid importing routev1.
func (r *ServerController) reconcileRoute(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling server route")

	host := getServerHost(cr)

	// Apply hostname shortening
	if host != "" {
		shortened, err := shortenHostname(host)
		if err != nil {
			return fmt.Errorf("failed to shorten hostname: %w", err)
		}
		host = shortened
	}

	// Determine TLS termination and target port
	tlsTermination := "reencrypt"
	targetPort := "https"

	if cr.Spec.Server.Insecure {
		tlsTermination = "edge"
		targetPort = "http"
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("TargetPort", targetPort).
		WithExtra("TLSTermination", tlsTermination)

	if host != "" {
		data.WithExtra("RouteHost", host)
	}

	// Add route-specific annotations
	if len(cr.Spec.Server.Route.Annotations) > 0 {
		data.WithAnnotations(cr.Spec.Server.Route.Annotations)
	}

	// WildcardPolicy
	if cr.Spec.Server.Route.WildcardPolicy != nil {
		data.WithExtra("WildcardPolicy", string(*cr.Spec.Server.Route.WildcardPolicy))
	}

	obj, err := r.templateEngine.RenderManifest("route.yaml.tmpl", data)
	if err != nil {
		return err
	}

	// The rendered object is already unstructured from the template engine
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("expected unstructured object from template engine")
	}

	// Ensure GVK is set correctly
	unstructuredObj.SetGroupVersionKind(routeGVK)

	return component.ReconcileUnstructuredResource(ctx, r.Client, r.Scheme, cr, unstructuredObj)
}

// cleanupIngressRoute removes both Ingress and Route resources
func (r *ServerController) cleanupIngressRoute(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	name := cr.Name + "-" + componentName

	// Clean up Ingress
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &networkingv1.Ingress{}); err != nil {
		log.Error(err, "failed to cleanup server ingress")
	}

	// Clean up Route (unstructured)
	if err := component.DeleteUnstructuredResourceIfExists(ctx, r.Client, name, cr.Namespace, routeGVK); err != nil {
		// Route API may not be available, which is fine
		log.V(1).Info("route cleanup skipped (API may not be available)", "error", err)
	}

	return nil
}

// shortenHostname shortens the hostname to comply with Kubernetes limits.
// Labels can be max 63 chars, total hostname max 253 chars.
func shortenHostname(hostname string) (string, error) {
	if len(hostname) <= 253 {
		// Check individual labels
		labels := strings.Split(hostname, ".")
		for _, label := range labels {
			if len(label) > 63 {
				break // Need to shorten
			}
		}
		if len(hostname) <= 253 {
			allOK := true
			for _, label := range labels {
				if len(label) > 63 {
					allOK = false
					break
				}
			}
			if allOK {
				return hostname, nil
			}
		}
	}

	labels := strings.Split(hostname, ".")
	if len(labels) == 0 {
		return hostname, nil
	}

	// Truncate first label if needed
	if len(labels[0]) > 63 {
		labels[0] = labels[0][:63]
	}

	// Verify other labels are within limits
	for i := 1; i < len(labels); i++ {
		if len(labels[i]) > 63 {
			return "", fmt.Errorf("label %q in hostname exceeds 63 character limit", labels[i])
		}
	}

	result := strings.Join(labels, ".")

	// Iteratively shorten first label if total is too long
	for len(result) > 253 && len(labels[0]) > 20 {
		labels[0] = labels[0][:len(labels[0])-1]
		result = strings.Join(labels, ".")
	}

	if len(result) > 253 {
		return "", fmt.Errorf("hostname %q exceeds 253 character limit after shortening", hostname)
	}

	return result, nil
}
