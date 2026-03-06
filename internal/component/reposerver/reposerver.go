package reposerver

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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

const componentName = "repo-server"

var log = logs.Log.WithName("RepoServerController")

//go:embed templates/*.tmpl
var templateFS embed.FS

// RepoServerController manages the Argo CD Repo Server component
type RepoServerController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewRepoServerController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *RepoServerController {
	return &RepoServerController{
		Client:         client,
		Scheme:         scheme,
		logger:         log,
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *RepoServerController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *RepoServerController) Name() string {
	return "repo-server"
}

func (r *RepoServerController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return cr.Spec.Repo.IsEnabled() && !cr.Spec.Repo.IsRemote()
}

func (r *RepoServerController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling repo-server component")

	// Reconcile repo server service account
	if err := r.reconcileRepoServerServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile repo server deployment
	if err := r.reconcileRepoServerDeployment(ctx, cr); err != nil {
		return err
	}

	// Reconcile repo server service
	if err := r.reconcileRepoServerService(ctx, cr); err != nil {
		return err
	}

	log.Info("repo-server component reconciliation complete")
	return nil
}

func (r *RepoServerController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("repo-server is not enabled or is remote, removing resources")
	name := cr.Name + "-" + componentName

	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &appsv1.Deployment{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.ServiceAccount{}); err != nil {
		return err
	}

	return nil
}

// reconcileRepoServerServiceAccount reconciles the ServiceAccount for the Repo Server
func (r *RepoServerController) reconcileRepoServerServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling repo-server serviceaccount")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("AutomountServiceAccountToken", cr.Spec.Repo.MountSAToken)

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

// reconcileRepoServerDeployment reconciles the Deployment for the Repo Server
func (r *RepoServerController) reconcileRepoServerDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling repo-server deployment")

	saName := cr.Name + "-" + componentName
	if cr.Spec.Repo.ServiceAccount != "" {
		saName = cr.Spec.Repo.ServiceAccount
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, componentName).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(saName).
		WithImage(getRepoServerContainerImage(cr)).
		WithExtra("InitImage", component.GetArgoContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("Command", getRepoServerCommand(cr)).
		WithExtra("AutomountServiceAccountToken", cr.Spec.Repo.MountSAToken)

	if cr.Spec.Repo.Replicas != nil {
		data.WithExtra("Replicas", *cr.Spec.Repo.Replicas)
	}

	resources := getRepoServerResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	if cr.Spec.Repo.ExecTimeout != nil {
		data.WithExtra("ExecTimeout", *cr.Spec.Repo.ExecTimeout)
	}

	if len(cr.Spec.Repo.Env) > 0 {
		data.WithExtra("Env", cr.Spec.Repo.Env)
	}

	if len(cr.Spec.Repo.Volumes) > 0 {
		data.WithExtra("Volumes", cr.Spec.Repo.Volumes)
	}
	if len(cr.Spec.Repo.VolumeMounts) > 0 {
		data.WithExtra("VolumeMounts", cr.Spec.Repo.VolumeMounts)
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

// reconcileRepoServerService reconciles the Service for the Repo Server
func (r *RepoServerController) reconcileRepoServerService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling repo-server service")

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
