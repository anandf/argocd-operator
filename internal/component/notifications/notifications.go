package notifications

import (
	"context"
	"embed"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

const componentName = "notifications-controller"

//go:embed templates/*.tmpl
var templateFS embed.FS

// NotificationsController manages the Argo CD Notifications Controller component
type NotificationsController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewNotificationsController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *NotificationsController {
	return &NotificationsController{
		Client:         client,
		Scheme:         scheme,
		logger:         logs.Log.WithName("NotificationsController"),
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *NotificationsController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *NotificationsController) Name() string {
	return "notifications"
}

func (r *NotificationsController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return r.isNotificationsEnabled(cr)
}

func (r *NotificationsController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller component")

	// Reconcile notifications controller service account
	if err := r.reconcileNotificationsServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile notifications controller role
	if err := r.reconcileNotificationsRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile notifications controller role binding
	if err := r.reconcileNotificationsRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile notifications controller deployment
	if err := r.reconcileNotificationsDeployment(ctx, cr); err != nil {
		return err
	}

	// Reconcile notifications controller service (metrics)
	if err := r.reconcileNotificationsService(ctx, cr); err != nil {
		return err
	}

	log.Info("notifications-controller component reconciliation complete")
	return nil
}

func (r *NotificationsController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("notifications-controller is not enabled, removing resources")
	return r.cleanupNotificationsResources(ctx, cr)
}

// isNotificationsEnabled checks if the notifications controller should be enabled
func (r *NotificationsController) isNotificationsEnabled(cr *argoproj.ArgoCD) bool {
	// Notifications controller is enabled by default.
	// It is considered disabled only if replicas is explicitly set to 0.
	if cr.Spec.Notifications.Replicas != nil && *cr.Spec.Notifications.Replicas == 0 {
		return false
	}
	return true
}

// cleanupNotificationsResources removes notifications controller resources when disabled
func (r *NotificationsController) cleanupNotificationsResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("cleaning up notifications-controller resources")
	name := cr.Name + "-" + componentName

	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name+"-metrics", cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.ServiceAccount{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &rbacv1.Role{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &rbacv1.RoleBinding{}); err != nil {
		return err
	}

	return nil
}

// reconcileNotificationsServiceAccount reconciles the ServiceAccount for Notifications Controller
func (r *NotificationsController) reconcileNotificationsServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller serviceaccount")

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

// reconcileNotificationsRole reconciles the Role for Notifications Controller
func (r *NotificationsController) reconcileNotificationsRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller role")

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

// reconcileNotificationsRoleBinding reconciles the RoleBinding for Notifications Controller
func (r *NotificationsController) reconcileNotificationsRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller rolebinding")

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

// reconcileNotificationsDeployment reconciles the Deployment for Notifications Controller
func (r *NotificationsController) reconcileNotificationsDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller deployment")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-" + componentName).
		WithImage(getNotificationsContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("Command", getNotificationsCommand(cr))

	resources := getNotificationsResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	if len(cr.Spec.Notifications.Env) > 0 {
		data.WithExtra("Env", cr.Spec.Notifications.Env)
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

// reconcileNotificationsService reconciles the metrics Service for Notifications Controller
func (r *NotificationsController) reconcileNotificationsService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling notifications-controller service")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

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
