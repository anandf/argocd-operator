package appsetcontroller

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	"github.com/argoproj/argo-cd/v3/util/glob"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	amerr "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ApplicationSetGitlabSCMTlsCertPath  = "/app/tls/scm/cert"
	ApplicationSetGitlabSCMTlsMountPath = "/app/tls/scm/"

	appSetComponent = "applicationset-controller"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

type ApplicationSetController struct {
	Client                                client.Client
	Scheme                                *runtime.Scheme
	ManagedApplicationSetSourceNamespaces map[string]string
	logger                                logr.Logger
	Decorators                            *decorator.DecoratorManager
	templateEngine                        *template.TemplateEngine
	config                                *component.ComponentConfig
}

func NewApplicationSetController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *ApplicationSetController {
	return &ApplicationSetController{
		Client:         client,
		Scheme:         scheme,
		logger:         logs.Log.WithName("ApplicationSetController"),
		Decorators:     decorators,
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *ApplicationSetController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *ApplicationSetController) Name() string {
	return "application-set"
}

func (r *ApplicationSetController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return cr.Spec.ApplicationSet != nil && cr.Spec.ApplicationSet.IsEnabled()
}

func (r *ApplicationSetController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)

	log.Info("reconciling applicationset serviceaccount")
	if err := r.reconcileServiceAccount(ctx, cr); err != nil {
		return err
	}

	log.Info("reconciling applicationset role")
	if err := r.reconcileRole(ctx, cr); err != nil {
		return err
	}

	log.Info("reconciling applicationset rolebinding")
	if err := r.reconcileRoleBinding(ctx, cr); err != nil {
		return err
	}

	log.Info("reconciling applicationset deployment")
	if err := r.reconcileDeployment(ctx, cr); err != nil {
		return err
	}

	log.Info("reconciling applicationset service")
	if err := r.reconcileService(ctx, cr); err != nil {
		return err
	}

	// create clusterrole & clusterrolebinding if cluster-scoped ArgoCD
	log.Info("reconciling applicationset clusterrole")
	if err := r.reconcileClusterRole(ctx, cr); err != nil {
		return err
	}

	log.Info("reconciling applicationset clusterrolebinding")
	if err := r.reconcileClusterRoleBinding(ctx, cr); err != nil {
		return err
	}

	// reconcile source namespace roles & rolebindings
	log.Info("reconciling applicationset roles & rolebindings in source namespaces")
	if err := r.reconcileSourceNamespacesResources(ctx, cr); err != nil {
		return err
	}

	// remove resources for namespaces not part of SourceNamespaces
	log.Info("performing cleanup for applicationset source namespaces")
	if err := r.removeUnmanagedSourceNamespaceResources(ctx, cr); err != nil {
		return err
	}

	return nil
}

func (r *ApplicationSetController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("application-set is not enabled, cleaning up resources")
	return r.cleanupResources(ctx, cr)
}

// cleanupResources removes all ApplicationSet controller resources
func (r *ApplicationSetController) cleanupResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	name := fmt.Sprintf("%s-%s", cr.Name, appSetComponent)

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

	// Cleanup cluster-scoped resources
	clusterResourceName := fmt.Sprintf("%s-%s-%s", cr.Name, cr.Namespace, common.ArgoCDApplicationSetControllerComponent)
	if err := component.DeleteResourceIfExists(ctx, r.Client, clusterResourceName, "", &rbacv1.ClusterRole{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, clusterResourceName, "", &rbacv1.ClusterRoleBinding{}); err != nil {
		return err
	}

	// Cleanup source namespace resources
	return r.removeUnmanagedSourceNamespaceResources(ctx, cr)
}

// newTemplateData creates common template data for appset resources
func (r *ApplicationSetController) newTemplateData(cr *argoproj.ArgoCD) *template.TemplateData {
	saName := fmt.Sprintf("%s-%s", cr.Name, appSetComponent)
	return template.NewTemplateData(cr, cr.Namespace, cr.Name, appSetComponent).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(saName)
}

func (r *ApplicationSetController) reconcileServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := r.newTemplateData(cr)

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

func (r *ApplicationSetController) reconcileRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := r.newTemplateData(cr)

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

func (r *ApplicationSetController) reconcileRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := r.newTemplateData(cr)

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

func (r *ApplicationSetController) reconcileDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := r.newTemplateData(cr).
		WithImage(component.GetApplicationSetContainerImage(cr)).
		WithExtra("Command", r.getCommand(cr))

	// Env vars
	appSetEnv := []corev1.EnvVar{{
		Name: "NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	}}
	appSetEnv = argoutil.EnvMerge(cr.Spec.ApplicationSet.Env, appSetEnv, true)
	appSetEnv = argoutil.EnvMerge(appSetEnv, component.ProxyEnvVars(), false)
	data.WithExtra("Env", appSetEnv)

	// Resources
	resources := component.GetApplicationSetResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	// Extra volumes and volume mounts from CR spec
	var extraVolumes []corev1.Volume
	var extraVolumeMounts []corev1.VolumeMount

	if cr.Spec.ApplicationSet.Volumes != nil {
		extraVolumes = append(extraVolumes, cr.Spec.ApplicationSet.Volumes...)
	}
	if cr.Spec.ApplicationSet.VolumeMounts != nil {
		extraVolumeMounts = append(extraVolumeMounts, cr.Spec.ApplicationSet.VolumeMounts...)
	}

	// SCM GitLab TLS cert volume
	if scmRootCAConfigMapName := r.getSCMRootCAConfigMapName(cr); scmRootCAConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: scmRootCAConfigMapName, Namespace: cr.Namespace}, cm); err == nil {
			extraVolumes = append(extraVolumes, corev1.Volume{
				Name: "appset-gitlab-scm-tls-cert",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: common.ArgoCDAppSetGitlabSCMTLSCertsConfigMapName,
						},
					},
				},
			})
			extraVolumeMounts = append(extraVolumeMounts, corev1.VolumeMount{
				Name:      "appset-gitlab-scm-tls-cert",
				MountPath: ApplicationSetGitlabSCMTlsMountPath,
			})
		}
	}

	if len(extraVolumes) > 0 {
		data.WithExtra("Volumes", extraVolumes)
	}
	if len(extraVolumeMounts) > 0 {
		data.WithExtra("VolumeMounts", extraVolumeMounts)
	}

	// Pod-level annotations and labels from CR spec
	if cr.Spec.ApplicationSet.Annotations != nil {
		data.WithExtra("PodAnnotations", cr.Spec.ApplicationSet.Annotations)
	}
	if cr.Spec.ApplicationSet.Labels != nil {
		data.WithExtra("PodLabels", cr.Spec.ApplicationSet.Labels)
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

func (r *ApplicationSetController) reconcileService(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := r.newTemplateData(cr)

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

// getCommand returns the command for the ArgoCD ApplicationSet controller.
func (r *ApplicationSetController) getCommand(cr *argoproj.ArgoCD) []string {
	log := r.logWithValues(cr)
	cmd := make([]string, 0)

	cmd = append(cmd, "entrypoint.sh")
	cmd = append(cmd, "argocd-applicationset-controller")

	if cr.Spec.Repo.IsEnabled() {
		cmd = append(cmd, "--argocd-repo-server", component.GetRepoServerAddress(cr))
	} else {
		log.Info("repo server is disabled, this would affect the functioning of ApplicationSet Controller")
	}

	cmd = append(cmd, "--loglevel")
	cmd = append(cmd, component.GetLogLevel(cr.Spec.ApplicationSet.LogLevel))

	cmd = append(cmd, "--logformat")
	cmd = append(cmd, component.GetLogFormat(cr.Spec.ApplicationSet.LogFormat))

	if cr.Spec.ApplicationSet.SCMRootCAConfigMap != "" {
		cmd = append(cmd, "--scm-root-ca-path")
		cmd = append(cmd, ApplicationSetGitlabSCMTlsCertPath)
	}

	// appset source namespaces should be subset of apps source namespaces
	appsetsSourceNamespaces := []string{}
	appsNamespaces, err := r.getSourceNamespaces(cr)
	if err == nil {
		for _, ns := range cr.Spec.ApplicationSet.SourceNamespaces {
			if component.Contains(appsNamespaces, ns) {
				appsetsSourceNamespaces = append(appsetsSourceNamespaces, ns)
			} else {
				log.V(1).Info("apps in target sourceNamespace is not enabled, skipping namespace in deployment command", "sourceNamespace", ns)
			}
		}
	}

	if len(appsetsSourceNamespaces) > 0 {
		cmd = append(cmd, "--applicationset-namespaces", fmt.Sprint(strings.Join(appsetsSourceNamespaces, ",")))
	}

	if len(cr.Spec.ApplicationSet.SCMProviders) > 0 {
		cmd = append(cmd, "--allowed-scm-providers", fmt.Sprint(strings.Join(cr.Spec.ApplicationSet.SCMProviders, ",")))
	}

	// appset in any ns is enabled and no scmProviders allow list is specified,
	// disables scm & PR generators to prevent potential security issues
	if len(appsetsSourceNamespaces) > 0 && !(len(cr.Spec.ApplicationSet.SCMProviders) > 0) {
		cmd = append(cmd, "--enable-scm-providers=false")
	}

	// ApplicationSet command arguments provided by the user
	extraArgs := cr.Spec.ApplicationSet.ExtraCommandArgs
	cmd = component.AppendUniqueArgs(cmd, extraArgs)

	return cmd
}

// getSCMRootCAConfigMapName returns the SCM root CA config map name if configured
func (r *ApplicationSetController) getSCMRootCAConfigMapName(cr *argoproj.ArgoCD) string {
	if cr.Spec.ApplicationSet != nil && cr.Spec.ApplicationSet.SCMRootCAConfigMap != "" {
		return cr.Spec.ApplicationSet.SCMRootCAConfigMap
	}
	return ""
}

// getSourceNamespaces retrieves a list of namespaces that match the sourceNamespaces
// pattern specified in the given ArgoCD
func (r *ApplicationSetController) getSourceNamespaces(cr *argoproj.ArgoCD) ([]string, error) {
	sourceNamespaces := []string{}
	namespaces := &corev1.NamespaceList{}

	if err := r.Client.List(context.TODO(), namespaces, &client.ListOptions{}); err != nil {
		return nil, err
	}

	for _, namespace := range namespaces.Items {
		if glob.MatchStringInList(cr.Spec.SourceNamespaces, namespace.Name, glob.REGEXP) {
			sourceNamespaces = append(sourceNamespaces, namespace.Name)
		}
	}

	return sourceNamespaces, nil
}

// reconcileClusterRole reconciles the ClusterRole for appset controller when ArgoCD is cluster-scoped
func (r *ApplicationSetController) reconcileClusterRole(ctx context.Context, cr *argoproj.ArgoCD) error {
	allowed := component.AllowedNamespace(cr.Namespace, os.Getenv("ARGOCD_CLUSTER_CONFIG_NAMESPACES"))

	clusterRoleName := fmt.Sprintf("%s-%s-%s", cr.Name, cr.Namespace, common.ArgoCDApplicationSetControllerComponent)

	if !allowed {
		// Cleanup any existing cluster role
		return component.DeleteResourceIfExists(ctx, r.Client, clusterRoleName, "", &rbacv1.ClusterRole{})
	}

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleName,
			Labels: argoutil.LabelsForCluster(cr),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"argoproj.io"},
				Resources: []string{"applications", "applicationsets"},
				Verbs:     []string{"list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"list", "watch"},
			},
		},
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, clusterRole, nil)
}

// reconcileClusterRoleBinding reconciles the ClusterRoleBinding for appset controller when ArgoCD is cluster-scoped
func (r *ApplicationSetController) reconcileClusterRoleBinding(ctx context.Context, cr *argoproj.ArgoCD) error {
	allowed := component.AllowedNamespace(cr.Namespace, os.Getenv("ARGOCD_CLUSTER_CONFIG_NAMESPACES"))

	clusterRBName := fmt.Sprintf("%s-%s-%s", cr.Name, cr.Namespace, common.ArgoCDApplicationSetControllerComponent)
	saName := fmt.Sprintf("%s-%s", cr.Name, appSetComponent)

	if !allowed {
		return component.DeleteResourceIfExists(ctx, r.Client, clusterRBName, "", &rbacv1.ClusterRoleBinding{})
	}

	clusterRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRBName,
			Labels: argoutil.LabelsForCluster(cr),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: cr.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRBName,
		},
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, clusterRB, nil)
}

// reconcileSourceNamespacesResources creates role & rolebinding in target source namespaces for appset controller
func (r *ApplicationSetController) reconcileSourceNamespacesResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	var reconciliationErrors []error

	if cr.Spec.ApplicationSet == nil {
		return nil
	}

	for _, sourceNamespace := range cr.Spec.ApplicationSet.SourceNamespaces {
		// source ns should be part of app-in-any-ns
		appsNamespaces, err := r.getSourceNamespaces(cr)
		if err != nil {
			reconciliationErrors = append(reconciliationErrors, err)
			continue
		}
		if !component.Contains(appsNamespaces, sourceNamespace) {
			log.Info("skipping reconciliation of resources, apps in target sourceNamespace is not enabled", "sourceNamespace", sourceNamespace)
			continue
		}

		// skip source ns if doesn't exist
		namespace := &corev1.Namespace{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: sourceNamespace}, namespace); err != nil {
			errMsg := fmt.Errorf("failed to retrieve namespace %s", sourceNamespace)
			reconciliationErrors = append(reconciliationErrors, errors.Join(errMsg, err))
			continue
		}

		// No namespace can be managed by multiple argo-cd instances
		if value, ok := namespace.Labels[common.ArgoCDManagedByLabel]; ok && value != "" {
			log.Info("skipping reconciling resources, namespace is already managed", "targetNamespace", namespace.Name, "managedBy", value)
			if val, ok1 := namespace.Labels[common.ArgoCDApplicationSetManagedByClusterArgoCDLabel]; ok1 && val != cr.Namespace {
				delete(r.ManagedApplicationSetSourceNamespaces, namespace.Name)
				if err := r.cleanupSourceNamespaceResources(ctx, cr, namespace.Name); err != nil {
					log.Error(err, "error cleaning up resources for namespace", "targetNamespace", namespace.Name)
				}
			}
			continue
		}

		log.Info("reconciling applicationset resources", "targetNamespace", namespace.Name)

		// add applicationset-managed-by-cluster-argocd label on namespace
		if _, ok := namespace.Labels[common.ArgoCDApplicationSetManagedByClusterArgoCDLabel]; !ok {
			if err := r.Client.Get(ctx, types.NamespacedName{Name: namespace.Name}, namespace); err != nil {
				return err
			}
			if namespace.Labels == nil {
				namespace.Labels = make(map[string]string)
			}
			namespace.Labels[common.ArgoCDApplicationSetManagedByClusterArgoCDLabel] = cr.Namespace
			if err := r.Client.Update(ctx, namespace); err != nil {
				log.Error(err, "failed to add label to namespace", "targetNamespace", namespace.Name)
			}
		}

		// role for applicationset controller in source namespace
		resourceName := component.GetResourceNameForApplicationSetSourceNamespaces(cr)
		saName := fmt.Sprintf("%s-%s", cr.Name, appSetComponent)

		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: sourceNamespace,
				Labels:    argoutil.LabelsForCluster(cr),
			},
			Rules: policyRuleForApplicationSetController(),
		}
		if err := r.reconcileSourceNamespaceRole(ctx, role); err != nil {
			reconciliationErrors = append(reconciliationErrors, err)
		}

		// rolebinding for applicationset controller in source namespace
		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:        resourceName,
				Labels:      argoutil.LabelsForCluster(cr),
				Annotations: argoutil.AnnotationsForCluster(cr),
				Namespace:   sourceNamespace,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     resourceName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      saName,
					Namespace: cr.Namespace,
				},
			},
		}
		if err := r.reconcileSourceNamespaceRoleBinding(ctx, roleBinding); err != nil {
			reconciliationErrors = append(reconciliationErrors, err)
		}

		if _, ok := r.ManagedApplicationSetSourceNamespaces[sourceNamespace]; !ok {
			if r.ManagedApplicationSetSourceNamespaces == nil {
				r.ManagedApplicationSetSourceNamespaces = make(map[string]string)
			}
			r.ManagedApplicationSetSourceNamespaces[sourceNamespace] = ""
		}
	}

	return amerr.NewAggregate(reconciliationErrors)
}

// policyRuleForApplicationSetController returns the policy rules for the appset controller in source namespaces
func policyRuleForApplicationSetController() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"applications", "applicationsets", "applicationsets/finalizers"},
			Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"applicationsets/status"},
			Verbs:     []string{"get", "patch", "update"},
		},
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"appprojects"},
			Verbs:     []string{"get"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"events"},
			Verbs:     []string{"create", "get", "list", "patch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}
}

func (r *ApplicationSetController) reconcileSourceNamespaceRole(ctx context.Context, role *rbacv1.Role) error {
	existingRole := &rbacv1.Role{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, existingRole)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to retrieve role %s in namespace %s: %w", role.Name, role.Namespace, err)
		}
		r.logger.Info("creating role", "name", role.Name, "namespace", role.Namespace)
		return r.Client.Create(ctx, role)
	}

	// Update rules if they differ
	existingRole.Rules = role.Rules
	r.logger.Info("updating role", "name", role.Name, "namespace", role.Namespace)
	return r.Client.Update(ctx, existingRole)
}

func (r *ApplicationSetController) reconcileSourceNamespaceRoleBinding(ctx context.Context, roleBinding *rbacv1.RoleBinding) error {
	existingRB := &rbacv1.RoleBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, existingRB)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to retrieve rolebinding %s in namespace %s: %w", roleBinding.Name, roleBinding.Namespace, err)
		}
		r.logger.Info("creating rolebinding", "name", roleBinding.Name, "namespace", roleBinding.Namespace)
		return r.Client.Create(ctx, roleBinding)
	}

	// RoleRef can't be updated - delete and recreate if it changed
	if existingRB.RoleRef != roleBinding.RoleRef {
		r.logger.Info("roleref changed, deleting rolebinding so it gets recreated", "name", roleBinding.Name, "namespace", roleBinding.Namespace)
		if err := r.Client.Delete(ctx, existingRB); err != nil {
			return err
		}
		return fmt.Errorf("change detected in roleRef for rolebinding %s, deleted for recreation", existingRB.Name)
	}

	// Update subjects if they differ
	existingRB.Subjects = roleBinding.Subjects
	r.logger.Info("updating rolebinding", "name", roleBinding.Name, "namespace", roleBinding.Namespace)
	return r.Client.Update(ctx, existingRB)
}

// removeUnmanagedSourceNamespaceResources cleans up resources from namespaces no longer managed
func (r *ApplicationSetController) removeUnmanagedSourceNamespaceResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	for ns := range r.ManagedApplicationSetSourceNamespaces {
		managedNamespace := false
		if cr.Spec.ApplicationSet != nil && cr.GetDeletionTimestamp() == nil {
			appsNamespaces, err := r.getSourceNamespaces(cr)
			if err != nil {
				return err
			}
			for _, namespace := range cr.Spec.ApplicationSet.SourceNamespaces {
				if namespace == ns && component.Contains(appsNamespaces, namespace) {
					managedNamespace = true
					break
				}
			}
		}

		if !managedNamespace {
			if err := r.cleanupSourceNamespaceResources(ctx, cr, ns); err != nil {
				log.Error(err, "error cleaning up applicationset resources for namespace", "targetNamespace", ns)
				continue
			}
			delete(r.ManagedApplicationSetSourceNamespaces, ns)
		}
	}
	return nil
}

// cleanupSourceNamespaceResources removes the application set resources from target namespace
func (r *ApplicationSetController) cleanupSourceNamespaceResources(ctx context.Context, cr *argoproj.ArgoCD, ns string) error {
	namespace := &corev1.Namespace{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: ns}, namespace); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	resourceName := component.GetResourceNameForApplicationSetSourceNamespaces(cr)

	if err := component.DeleteResourceIfExists(ctx, r.Client, resourceName, ns, &rbacv1.Role{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, resourceName, ns, &rbacv1.RoleBinding{}); err != nil {
		return err
	}

	// Remove applicationset-managed-by-cluster-argocd label from the namespace
	delete(namespace.Labels, common.ArgoCDApplicationSetManagedByClusterArgoCDLabel)
	if err := r.Client.Update(ctx, namespace); err != nil {
		return fmt.Errorf("failed to remove applicationset label from namespace %s: %w", namespace.Name, err)
	}

	return nil
}

// SetManagedApplicationSetSourceNamespaces populates ManagedApplicationSetSourceNamespaces
// with namespaces that have the "argocd.argoproj.io/applicationset-managed-by-cluster-argocd" label.
func (r *ApplicationSetController) SetManagedApplicationSetSourceNamespaces(cr *argoproj.ArgoCD) error {
	if r.ManagedApplicationSetSourceNamespaces == nil {
		r.ManagedApplicationSetSourceNamespaces = make(map[string]string)
	}
	namespaces := &corev1.NamespaceList{}
	listOption := client.MatchingLabels{
		common.ArgoCDApplicationSetManagedByClusterArgoCDLabel: cr.Namespace,
	}

	if err := r.Client.List(context.TODO(), namespaces, listOption); err != nil {
		return err
	}

	for _, namespace := range namespaces.Items {
		r.ManagedApplicationSetSourceNamespaces[namespace.Name] = ""
	}

	return nil
}
