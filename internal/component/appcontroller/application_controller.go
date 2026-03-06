package appcontroller

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

const componentName = "application-controller"

// ApplicationController manages the Argo CD Application Controller component

// This directive tells Go to include the 'templates' folder in the binary
//
//go:embed templates/*.tmpl
var templateFS embed.FS

type ApplicationController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewApplicationController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *ApplicationController {
	return &ApplicationController{
		Client:         client,
		Scheme:         scheme,
		logger:         logs.Log.WithName("ApplicationController"),
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *ApplicationController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *ApplicationController) Name() string {
	return componentName
}

func (r *ApplicationController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return cr.Spec.Controller.IsEnabled()
}

func (r *ApplicationController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller component")

	// Reconcile application controller service account
	if err := r.reconcileApplicationControllerServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller role
	if err := r.reconcileApplicationControllerRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller role binding
	if err := r.reconcileApplicationControllerRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller cluster role
	if err := r.reconcileApplicationControllerClusterRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller cluster role binding
	if err := r.reconcileApplicationControllerClusterRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller statefulset
	if err := r.reconcileApplicationControllerStatefulSet(ctx, cr); err != nil {
		return err
	}

	// Reconcile application controller service (metrics)
	if err := r.reconcileApplicationControllerService(ctx, cr); err != nil {
		return err
	}

	log.Info("application-controller component reconciliation complete")
	return nil
}

func (r *ApplicationController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("application-controller is not enabled, removing resources")
	name := cr.Name + "-" + componentName

	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &appsv1.StatefulSet{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name+"-metrics", cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.ServiceAccount{}); err != nil {
		return err
	}

	// Namespaced RBAC
	roleName := cr.Name + "-argocd-" + componentName
	if err := component.DeleteResourceIfExists(ctx, r.Client, roleName, cr.Namespace, &rbacv1.Role{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, roleName, cr.Namespace, &rbacv1.RoleBinding{}); err != nil {
		return err
	}

	// Cluster-scoped RBAC (cluster role/binding names use CR name prefix)
	clusterRoleName := cr.Name + "-argocd-" + componentName
	if err := component.DeleteResourceIfExists(ctx, r.Client, clusterRoleName, "", &rbacv1.ClusterRole{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, clusterRoleName, "", &rbacv1.ClusterRoleBinding{}); err != nil {
		return err
	}

	return nil
}

// reconcileApplicationControllerServiceAccount reconciles the ServiceAccount for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller serviceaccount")

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

// reconcileApplicationControllerRole reconciles the Role for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller role")

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

// reconcileApplicationControllerRoleBinding reconciles the RoleBinding for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller rolebinding")

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

// reconcileApplicationControllerClusterRole reconciles the ClusterRole for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerClusterRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller clusterrole")

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

	// ClusterRole is cluster-scoped, so ReconcileResource won't set owner ref
	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, clusterRole, nil)
}

// reconcileApplicationControllerClusterRoleBinding reconciles the ClusterRoleBinding for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerClusterRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller clusterrolebinding")

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

	// ClusterRoleBinding is cluster-scoped, so ReconcileResource won't set owner ref
	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, crb, nil)
}

// reconcileApplicationControllerStatefulSet reconciles the StatefulSet for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerStatefulSet(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller statefulset")

	replicas := int32(1)
	if cr.Spec.Controller.Sharding.Enabled {
		replicas = cr.Spec.Controller.Sharding.Replicas
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-argocd-" + componentName).
		WithImage(component.GetArgoContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("Replicas", replicas)

	resources := getApplicationControllerResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	component.ApplyNodePlacement(data, cr.Spec.NodePlacement)

	obj, err := r.templateEngine.RenderManifest("statefulset.yaml.tmpl", data)
	if err != nil {
		return err
	}

	sts := &appsv1.StatefulSet{}
	if err := template.ConvertToTyped(obj, sts); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, sts, r.Decorators)
}

// reconcileApplicationControllerService reconciles the metrics Service for the Application Controller
func (r *ApplicationController) reconcileApplicationControllerService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling application-controller service")

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

// getApplicationControllerResources returns the ResourceRequirements for the Application Controller
func getApplicationControllerResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.Controller.Resources != nil {
		resources = *cr.Spec.Controller.Resources
	}
	return resources
}
