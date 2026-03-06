package dex

import (
	"context"
	"embed"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	"github.com/argoproj-labs/argocd-operator/internal/template"
)

var log = logs.Log.WithName("DexController")

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	// ArgoCDDefaultDexSuffix is the default suffix to use for Dex resources.
	ArgoCDDefaultDexSuffix = "dex"
)

// DexController manages the Dex SSO component for Argo CD
type DexController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewDexController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *DexController {
	return &DexController{
		Client:         client,
		Scheme:         scheme,
		logger:         log,
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *DexController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *DexController) Name() string {
	return "dex"
}

func (r *DexController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return r.isDexEnabled(cr)
}

func (r *DexController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex component")

	// Reconcile dex service account
	if err := r.reconcileDexServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile dex role
	if err := r.reconcileDexRole(ctx, cr); err != nil {
		return err
	}

	// Reconcile dex role binding
	if err := r.reconcileDexRoleBinding(ctx, cr); err != nil {
		return err
	}

	// Reconcile dex deployment
	if err := r.reconcileDexDeployment(ctx, cr); err != nil {
		return err
	}

	// Reconcile dex service
	if err := r.reconcileDexService(ctx, cr); err != nil {
		return err
	}

	log.Info("dex component reconciliation complete")
	return nil
}

func (r *DexController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("dex is not enabled, removing resources")
	return r.cleanupDexResources(ctx, cr)
}

// isDexEnabled checks if Dex should be enabled based on the ArgoCD CR
func (r *DexController) isDexEnabled(cr *argoproj.ArgoCD) bool {
	if cr.Spec.SSO != nil {
		return cr.Spec.SSO.Provider.ToLower() == argoproj.SSOProviderTypeDex
	}
	return false
}

// cleanupDexResources removes Dex resources when Dex is disabled
func (r *DexController) cleanupDexResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("cleaning up dex resources")
	name := cr.Name + "-dex-server"

	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.Service{}); err != nil {
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

// reconcileDexServiceAccount reconciles the ServiceAccount for the Dex component
func (r *DexController) reconcileDexServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex serviceaccount")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, ArgoCDDefaultDexSuffix).
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

// reconcileDexRole reconciles the Role for the Dex component
func (r *DexController) reconcileDexRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex role")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, ArgoCDDefaultDexSuffix).
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

// reconcileDexRoleBinding reconciles the RoleBinding for the Dex component
func (r *DexController) reconcileDexRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex rolebinding")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, ArgoCDDefaultDexSuffix).
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

// reconcileDexDeployment reconciles the Deployment for the Dex component
func (r *DexController) reconcileDexDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex deployment")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, ArgoCDDefaultDexSuffix).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name+"-dex-server").
		WithImage(getDexContainerImage(cr)).
		WithExtra("InitImage", component.GetArgoContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy)))

	resources := getDexResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	// Dex-specific env vars
	if cr.Spec.SSO != nil && cr.Spec.SSO.Dex != nil {
		if len(cr.Spec.SSO.Dex.Env) > 0 {
			data.WithExtra("Env", cr.Spec.SSO.Dex.Env)
		}
		if len(cr.Spec.SSO.Dex.Volumes) > 0 {
			data.WithExtra("Volumes", cr.Spec.SSO.Dex.Volumes)
		}
		if len(cr.Spec.SSO.Dex.VolumeMounts) > 0 {
			data.WithExtra("VolumeMounts", cr.Spec.SSO.Dex.VolumeMounts)
		}
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

// reconcileDexService reconciles the Service for the Dex component
func (r *DexController) reconcileDexService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling dex service")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, ArgoCDDefaultDexSuffix).
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
