package component

import (
	"context"
	"embed"
	"fmt"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/pkg/component/template"
	"github.com/argoproj-labs/argocd-operator/pkg/platform"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

//go:embed manifests
var manifestsFS embed.FS

// TemplateBasedController is a generic controller that uses templates to create resources
type TemplateBasedController struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	Component        string
	TemplateRoot     string
	PlatformType     string // "kubernetes" or "openshift"
	DecoratorManager *DecoratorManager
	logger           logr.Logger
	engine           *template.TemplateEngine
}

// NewTemplateBasedController creates a new template-based controller
func NewTemplateBasedController(
	client client.Client,
	scheme *runtime.Scheme,
	component string,
	platformType string,
) *TemplateBasedController {
	return &TemplateBasedController{
		Client:           client,
		Scheme:           scheme,
		Component:        component,
		TemplateRoot:     "manifests",
		PlatformType:     platformType,
		DecoratorManager: nil, // Set by caller if needed
		logger:           logs.Log.WithName(fmt.Sprintf("%sTemplateController", component)),
		engine:           template.NewTemplateEngine(manifestsFS, "manifests"),
	}
}

// WithDecorators sets the decorator manager for this controller
func (r *TemplateBasedController) WithDecorators(decorators *DecoratorManager) *TemplateBasedController {
	r.DecoratorManager = decorators
	return r
}

// Reconcile reconciles the component using templates
func (r *TemplateBasedController) Reconcile(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error {
	r.logger.Info("reconciling component using templates", "component", r.Component)

	// Build template data
	data := r.buildTemplateData(cr, apiDetector)

	// Render base templates
	if err := r.reconcileBaseResources(cr, data); err != nil {
		return fmt.Errorf("failed to reconcile base resources: %w", err)
	}

	// Render platform-specific templates
	if err := r.reconcilePlatformResources(cr, data, apiDetector); err != nil {
		return fmt.Errorf("failed to reconcile platform resources: %w", err)
	}

	r.logger.Info("component reconciliation complete", "component", r.Component)
	return nil
}

// buildTemplateData builds the template data for rendering
func (r *TemplateBasedController) buildTemplateData(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) *template.TemplateData {
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, r.Component)

	// Add common labels
	data.WithLabels(map[string]string{
		common.ArgoCDKeyName:      cr.Name,
		common.ArgoCDKeyComponent: r.Component,
		"app.kubernetes.io/name":    fmt.Sprintf("argocd-%s", r.Component),
		"app.kubernetes.io/instance": cr.Name,
		"app.kubernetes.io/component": r.Component,
		"app.kubernetes.io/part-of":   "argocd",
	})

	// Add service account
	data.WithServiceAccount(fmt.Sprintf("%s-%s", cr.Name, r.Component))

	// Add component-specific data
	r.addComponentSpecificData(cr, data, apiDetector)

	return data
}

// addComponentSpecificData adds component-specific data to the template
func (r *TemplateBasedController) addComponentSpecificData(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) {
	switch r.Component {
	case "server":
		r.addServerData(cr, data, apiDetector)
	case "repo-server":
		r.addRepoServerData(cr, data)
	case "application-controller":
		r.addApplicationControllerData(cr, data)
	case "applicationset-controller":
		r.addApplicationSetData(cr, data)
	case "redis":
		r.addRedisData(cr, data)
	case "dex":
		r.addDexData(cr, data)
	case "notifications-controller":
		r.addNotificationsData(cr, data)
	}
}

// addServerData adds server-specific data
func (r *TemplateBasedController) addServerData(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) {
	// Set image
	serverImage := getArgoServerImage(cr)
	data.WithImage(serverImage)

	// Add extra data
	data.WithExtra("Replicas", getArgoServerReplicas(cr))
	data.WithExtra("ImagePullPolicy", "Always")

	// Add resources if specified
	if cr.Spec.Server.Resources != nil {
		data.WithExtra("Resources", map[string]interface{}{
			"Limits":   cr.Spec.Server.Resources.Limits,
			"Requests": cr.Spec.Server.Resources.Requests,
		})
	}

	// Add environment variables
	if len(cr.Spec.Server.Env) > 0 {
		data.WithExtra("Env", cr.Spec.Server.Env)
	}

	// Add volumes and volume mounts
	if len(cr.Spec.Server.Volumes) > 0 {
		data.WithExtra("Volumes", cr.Spec.Server.Volumes)
	}
	if len(cr.Spec.Server.VolumeMounts) > 0 {
		data.WithExtra("VolumeMounts", cr.Spec.Server.VolumeMounts)
	}

	// Add node selector and tolerations
	if len(cr.Spec.Server.NodeSelector) > 0 {
		data.WithExtra("NodeSelector", cr.Spec.Server.NodeSelector)
	}
	if len(cr.Spec.Server.Tolerations) > 0 {
		data.WithExtra("Tolerations", cr.Spec.Server.Tolerations)
	}

	// Add service type
	data.WithExtra("ServiceType", cr.Spec.Server.Service.Type)

	// Add ingress/route configuration
	if cr.Spec.Server.Ingress.Enabled {
		data.WithExtra("IngressEnabled", true)
		data.WithExtra("IngressHost", cr.Spec.Server.Ingress.Host)
		data.WithExtra("IngressClassName", cr.Spec.Server.Ingress.IngressClassName)
		data.WithExtra("IngressAnnotations", cr.Spec.Server.Ingress.Annotations)
		if len(cr.Spec.Server.Ingress.TLS) > 0 {
			data.WithExtra("TLS", cr.Spec.Server.Ingress.TLS)
		}
	}

	if cr.Spec.Server.Route.Enabled {
		data.WithExtra("RouteEnabled", true)
		data.WithExtra("RouteHost", cr.Spec.Server.Route.Host)
		data.WithExtra("RouteAnnotations", cr.Spec.Server.Route.Annotations)
		data.WithExtra("TLSTermination", cr.Spec.Server.Route.TLS.Termination)
		data.WithExtra("TLSInsecureEdgeTerminationPolicy", cr.Spec.Server.Route.TLS.InsecureEdgeTerminationPolicy)
	}
}

// Placeholder functions for other components
func (r *TemplateBasedController) addRepoServerData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add repo-server specific data
}

func (r *TemplateBasedController) addApplicationControllerData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add application-controller specific data
}

func (r *TemplateBasedController) addApplicationSetData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add applicationset-controller specific data
}

func (r *TemplateBasedController) addRedisData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add redis specific data
}

func (r *TemplateBasedController) addDexData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add dex specific data
}

func (r *TemplateBasedController) addNotificationsData(cr *argoproj.ArgoCD, data *template.TemplateData) {
	// TODO: Add notifications-controller specific data
}

// reconcileBaseResources reconciles the base resources (ServiceAccount, Role, RoleBinding, Deployment, Service)
func (r *TemplateBasedController) reconcileBaseResources(cr *argoproj.ArgoCD, data *template.TemplateData) error {
	ctx := context.Background()
	basePath := fmt.Sprintf("base/%s", r.Component)

	// Define the base resources in order
	baseResources := []struct {
		templatePath string
		objType      client.Object
	}{
		{"serviceaccount.yaml", &corev1.ServiceAccount{}},
		{"role.yaml", &rbacv1.Role{}},
		{"rolebinding.yaml", &rbacv1.RoleBinding{}},
		{"deployment.yaml", &appsv1.Deployment{}},
		{"service.yaml", &corev1.Service{}},
		{"service-metrics.yaml", &corev1.Service{}},
	}

	for _, res := range baseResources {
		templatePath := fmt.Sprintf("%s/%s", basePath, res.templatePath)

		// Render the template
		obj, err := r.engine.RenderManifest(templatePath, data)
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", templatePath, err)
		}

		// Convert to typed object
		typedObj := res.objType.DeepCopyObject().(client.Object)
		if err := template.ConvertToTyped(obj, typedObj); err != nil {
			return fmt.Errorf("failed to convert %s to typed object: %w", templatePath, err)
		}

		// Reconcile the resource (decorators are applied inside reconcileResource)
		if err := r.reconcileResource(ctx, cr, typedObj, r.DecoratorManager); err != nil {
			return fmt.Errorf("failed to reconcile %s: %w", templatePath, err)
		}
	}

	return nil
}

// reconcilePlatformResources reconciles platform-specific resources (Ingress for Kubernetes, Route for OpenShift)
func (r *TemplateBasedController) reconcilePlatformResources(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) error {
	ctx := context.Background()

	// Check if ingress/route is enabled
	if r.Component == "server" {
		// Handle Kubernetes Ingress
		if cr.Spec.Server.Ingress.Enabled && apiDetector.HasIngress(ctx) {
			templatePath := fmt.Sprintf("kubernetes/%s/ingress.yaml", r.Component)
			obj, err := r.engine.RenderManifest(templatePath, data)
			if err != nil {
				return fmt.Errorf("failed to render ingress: %w", err)
			}

			ingress := &networkingv1.Ingress{}
			if err := template.ConvertToTyped(obj, ingress); err != nil {
				return fmt.Errorf("failed to convert ingress to typed object: %w", err)
			}

			if err := r.reconcileResource(ctx, cr, ingress, r.DecoratorManager); err != nil {
				return fmt.Errorf("failed to reconcile ingress: %w", err)
			}
		}

		// Handle OpenShift Route
		if cr.Spec.Server.Route.Enabled && apiDetector.HasRoute(ctx) {
			templatePath := fmt.Sprintf("openshift/%s/route.yaml", r.Component)
			obj, err := r.engine.RenderManifest(templatePath, data)
			if err != nil {
				return fmt.Errorf("failed to render route: %w", err)
			}

			// Route is handled as unstructured since it's OpenShift-specific
			if err := r.reconcileResource(ctx, cr, obj, r.DecoratorManager); err != nil {
				return fmt.Errorf("failed to reconcile route: %w", err)
			}
		}
	}

	return nil
}

// reconcileResource reconciles a single resource (create or update)
func (r *TemplateBasedController) reconcileResource(ctx context.Context, cr *argoproj.ArgoCD, obj client.Object, decoratorManager *DecoratorManager) error {
	// Apply decorators before creating/updating
	if decoratorManager != nil {
		if err := decoratorManager.Decorate(obj); err != nil {
			return fmt.Errorf("failed to apply decorators: %w", err)
		}
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(cr, obj, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if resource exists
	existing := obj.DeepCopyObject().(client.Object)
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, existing)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create the resource
			r.logger.Info("creating resource",
				"kind", obj.GetObjectKind().GroupVersionKind().Kind,
				"name", obj.GetName())
			return r.Client.Create(ctx, obj)
		}
		return err
	}

	// Update the resource
	r.logger.Info("updating resource",
		"kind", obj.GetObjectKind().GroupVersionKind().Kind,
		"name", obj.GetName())

	// Preserve resource version for update
	obj.SetResourceVersion(existing.GetResourceVersion())
	return r.Client.Update(ctx, obj)
}

// Helper functions (these should be extracted from the legacy code)
func getArgoServerImage(cr *argoproj.ArgoCD) string {
	// TODO: Extract from legacy code
	return common.ArgoCDDefaultArgoImage
}

func getArgoServerReplicas(cr *argoproj.ArgoCD) int32 {
	if cr.Spec.Server.Replicas != nil {
		return *cr.Spec.Server.Replicas
	}
	return 1
}
