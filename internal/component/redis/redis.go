package redis

import (
	"context"
	"embed"
	"fmt"

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

var log = logs.Log.WithName("RedisController")

//go:embed templates/*.tmpl
var templateFS embed.FS

// RedisController manages the Redis component for Argo CD
type RedisController struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	logger         logr.Logger
	templateEngine *template.TemplateEngine
	Decorators     *decorator.DecoratorManager
	config         *component.ComponentConfig
}

func NewRedisController(client client.Client, scheme *runtime.Scheme, decorators *decorator.DecoratorManager, opts ...component.ComponentOption) *RedisController {
	return &RedisController{
		Client:         client,
		Scheme:         scheme,
		logger:         log,
		templateEngine: template.NewTemplateEngine(templateFS, "templates"),
		Decorators:     decorators,
		config:         component.NewComponentConfig(opts...),
	}
}

func (r *RedisController) logWithValues(cr *argoproj.ArgoCD) logr.Logger {
	return r.logger.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
}

func (r *RedisController) Name() string {
	return "redis"
}

func (r *RedisController) IsEnabled(cr *argoproj.ArgoCD) bool {
	return r.isRedisEnabled(cr)
}

func (r *RedisController) Ensure(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis component")

	// Determine if Redis HA mode is enabled
	isHA := r.isRedisHAEnabled(cr)

	if isHA {
		log.Info("reconciling redis in HA mode")
		return r.reconcileRedisHA(ctx, cr)
	}

	log.Info("reconciling redis in standalone mode")
	return r.reconcileRedisStandalone(ctx, cr)
}

func (r *RedisController) Remove(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("redis is not enabled (external redis configured), removing resources")
	return r.cleanupRedisResources(ctx, cr)
}

// isRedisEnabled checks if Redis should be deployed (vs using external Redis)
func (r *RedisController) isRedisEnabled(cr *argoproj.ArgoCD) bool {
	// If external Redis is configured, don't deploy internal Redis
	if cr.Spec.Redis.Remote != nil && *cr.Spec.Redis.Remote != "" {
		return false
	}
	return true
}

// isRedisHAEnabled checks if Redis HA mode is enabled
func (r *RedisController) isRedisHAEnabled(cr *argoproj.ArgoCD) bool {
	return cr.Spec.HA.Enabled
}

// cleanupRedisResources removes standalone Redis resources
func (r *RedisController) cleanupRedisResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("cleaning up redis resources")
	name := cr.Name + "-" + common.ArgoCDDefaultRedisSuffix

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

// cleanupRedisHAResources removes HA Redis resources when switching to standalone mode
func (r *RedisController) cleanupRedisHAResources(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("cleaning up redis HA resources")

	// StatefulSet
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha-server", cr.Namespace, &appsv1.StatefulSet{}); err != nil {
		return err
	}
	// HAProxy Deployment
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha-haproxy", cr.Namespace, &appsv1.Deployment{}); err != nil {
		return err
	}
	// Services
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha", cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha-haproxy", cr.Namespace, &corev1.Service{}); err != nil {
		return err
	}
	for i := int32(0); i < getRedisHAReplicas(); i++ {
		name := fmt.Sprintf("%s-redis-ha-announce-%d", cr.Name, i)
		if err := component.DeleteResourceIfExists(ctx, r.Client, name, cr.Namespace, &corev1.Service{}); err != nil {
			return err
		}
	}
	// ConfigMaps
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha-configmap", cr.Namespace, &corev1.ConfigMap{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, cr.Name+"-redis-ha-health-configmap", cr.Namespace, &corev1.ConfigMap{}); err != nil {
		return err
	}
	// HA RBAC
	haName := cr.Name + "-redis-ha"
	if err := component.DeleteResourceIfExists(ctx, r.Client, haName, cr.Namespace, &rbacv1.Role{}); err != nil {
		return err
	}
	if err := component.DeleteResourceIfExists(ctx, r.Client, haName, cr.Namespace, &rbacv1.RoleBinding{}); err != nil {
		return err
	}

	return nil
}

// reconcileRedisStandalone reconciles Redis in standalone mode
func (r *RedisController) reconcileRedisStandalone(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling standalone redis")

	// Clean up HA resources if switching from HA to standalone
	if err := r.cleanupRedisHAResources(ctx, cr); err != nil {
		log.Error(err, "failed to cleanup redis HA resources")
	}

	// Reconcile redis service account
	if err := r.reconcileRedisServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis role
	if err := r.reconcileRedisRole(ctx, cr, false); err != nil {
		return err
	}

	// Reconcile redis role binding
	if err := r.reconcileRedisRoleBinding(ctx, cr, false); err != nil {
		return err
	}

	// Reconcile redis deployment
	if err := r.reconcileRedisDeployment(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis service
	if err := r.reconcileRedisService(ctx, cr); err != nil {
		return err
	}

	log.Info("standalone redis reconciliation complete")
	return nil
}

// reconcileRedisHA reconciles Redis in HA mode (StatefulSet + Sentinel)
func (r *RedisController) reconcileRedisHA(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA")

	// Clean up standalone resources if switching from standalone to HA
	if err := r.cleanupRedisResources(ctx, cr); err != nil {
		log.Error(err, "failed to cleanup standalone redis resources")
	}

	// Reconcile redis service account (shared between standalone and HA)
	if err := r.reconcileRedisHAServiceAccount(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis HA role
	if err := r.reconcileRedisRole(ctx, cr, true); err != nil {
		return err
	}

	// Reconcile redis HA role binding
	if err := r.reconcileRedisRoleBinding(ctx, cr, true); err != nil {
		return err
	}

	// Reconcile redis HA ConfigMap
	if err := r.reconcileRedisHAConfigMap(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis HA health check ConfigMap
	if err := r.reconcileRedisHAHealthConfigMap(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis HA StatefulSet
	if err := r.reconcileRedisHAStatefulSet(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis HA services
	if err := r.reconcileRedisHAServices(ctx, cr); err != nil {
		return err
	}

	// Reconcile redis HA Proxy deployment
	if err := r.reconcileRedisHAProxyDeployment(ctx, cr); err != nil {
		return err
	}

	log.Info("redis HA reconciliation complete")
	return nil
}

// reconcileRedisServiceAccount reconciles the ServiceAccount for standalone Redis
func (r *RedisController) reconcileRedisServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis serviceaccount")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, common.ArgoCDDefaultRedisSuffix).
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

// reconcileRedisHAServiceAccount reconciles the ServiceAccount for Redis HA
func (r *RedisController) reconcileRedisHAServiceAccount(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA serviceaccount")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
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

	// Override the name to match HA pattern
	sa.Name = cr.Name + "-redis-ha"

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, sa, nil)
}

// reconcileRedisRole reconciles the Role for Redis (standalone or HA)
func (r *RedisController) reconcileRedisRole(ctx context.Context, cr *argoproj.ArgoCD, isHA bool) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis role", "ha", isHA)

	suffix := common.ArgoCDDefaultRedisSuffix
	if isHA {
		suffix = "redis-ha"
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, suffix).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("IsHA", isHA)

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

// reconcileRedisRoleBinding reconciles the RoleBinding for Redis (standalone or HA)
func (r *RedisController) reconcileRedisRoleBinding(ctx context.Context, cr *argoproj.ArgoCD, isHA bool) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis rolebinding", "ha", isHA)

	suffix := common.ArgoCDDefaultRedisSuffix
	if isHA {
		suffix = "redis-ha"
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, suffix).
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

// reconcileRedisDeployment reconciles the Deployment for standalone Redis
func (r *RedisController) reconcileRedisDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis deployment")

	useTLS := redisShouldUseTLS(r.Client, cr)

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, common.ArgoCDDefaultRedisSuffix).
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-" + common.ArgoCDDefaultRedisSuffix).
		WithImage(getRedisContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("UseTLS", useTLS).
		WithExtra("TLSSecretName", common.ArgoCDRedisServerTLSSecretName)

	resources := getRedisResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
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

// reconcileRedisService reconciles the Service for Redis
func (r *RedisController) reconcileRedisService(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis service")

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, common.ArgoCDDefaultRedisSuffix).
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

// reconcileRedisHAConfigMap reconciles the ConfigMap for Redis HA configuration
func (r *RedisController) reconcileRedisHAConfigMap(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA configmap")

	useTLS := redisShouldUseTLS(r.Client, cr)
	serviceName := cr.Name + "-redis-ha"
	params := scriptParams{
		ServiceName: serviceName,
		UseTLS:      tlsString(useTLS),
	}

	// Render all configuration scripts
	haproxyConfig, err := renderConfigScript("haproxy.cfg.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render haproxy config: %w", err)
	}

	haproxyInitScript, err := renderConfigScript("haproxy_init.sh.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render haproxy init script: %w", err)
	}

	initScript, err := renderConfigScript("init.sh.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render init script: %w", err)
	}

	redisConf, err := renderConfigScript("redis.conf.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render redis conf: %w", err)
	}

	sentinelConf, err := renderConfigScript("sentinel.conf.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render sentinel conf: %w", err)
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("HAProxyConfig", haproxyConfig).
		WithExtra("HAProxyInitScript", haproxyInitScript).
		WithExtra("InitScript", initScript).
		WithExtra("RedisConf", redisConf).
		WithExtra("SentinelConf", sentinelConf)

	obj, err := r.templateEngine.RenderManifest("configmap-ha.yaml.tmpl", data)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := template.ConvertToTyped(obj, cm); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, cm, nil)
}

// reconcileRedisHAHealthConfigMap reconciles the health check ConfigMap for Redis HA
func (r *RedisController) reconcileRedisHAHealthConfigMap(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA health configmap")

	useTLS := redisShouldUseTLS(r.Client, cr)
	params := scriptParams{
		UseTLS: tlsString(useTLS),
	}

	redisLiveness, err := renderConfigScript("redis_liveness.sh.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render redis liveness script: %w", err)
	}

	redisReadiness, err := renderConfigScript("redis_readiness.sh.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render redis readiness script: %w", err)
	}

	sentinelLiveness, err := renderConfigScript("sentinel_liveness.sh.tpl", params)
	if err != nil {
		return fmt.Errorf("failed to render sentinel liveness script: %w", err)
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("RedisLivenessScript", redisLiveness).
		WithExtra("RedisReadinessScript", redisReadiness).
		WithExtra("SentinelLivenessScript", sentinelLiveness)

	obj, err := r.templateEngine.RenderManifest("configmap-ha-health.yaml.tmpl", data)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := template.ConvertToTyped(obj, cm); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, cm, nil)
}

// reconcileRedisHAStatefulSet reconciles the StatefulSet for Redis HA
func (r *RedisController) reconcileRedisHAStatefulSet(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA statefulset")

	useTLS := redisShouldUseTLS(r.Client, cr)

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-redis-ha").
		WithImage(getRedisHAContainerImage(cr)).
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("UseTLS", useTLS).
		WithExtra("TLSSecretName", common.ArgoCDRedisServerTLSSecretName).
		WithExtra("Replicas", getRedisHAReplicas())

	resources := getRedisHAResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	component.ApplyNodePlacement(data, cr.Spec.NodePlacement)

	obj, err := r.templateEngine.RenderManifest("statefulset-ha.yaml.tmpl", data)
	if err != nil {
		return err
	}

	sts := &appsv1.StatefulSet{}
	if err := template.ConvertToTyped(obj, sts); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, sts, r.Decorators)
}

// reconcileRedisHAServices reconciles the Services for Redis HA
func (r *RedisController) reconcileRedisHAServices(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA services")

	// Reconcile headless master service
	if err := r.reconcileRedisHAMasterService(ctx, cr); err != nil {
		return err
	}

	// Reconcile announce services (one per replica)
	replicas := getRedisHAReplicas()
	for i := int32(0); i < replicas; i++ {
		if err := r.reconcileRedisHAAnnounceService(ctx, cr, i); err != nil {
			return err
		}
	}

	// Reconcile HAProxy service
	if err := r.reconcileRedisHAProxyService(ctx, cr); err != nil {
		return err
	}

	return nil
}

// reconcileRedisHAMasterService reconciles the headless master Service for Redis HA
func (r *RedisController) reconcileRedisHAMasterService(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("service-ha-master.yaml.tmpl", data)
	if err != nil {
		return err
	}

	svc := &corev1.Service{}
	if err := template.ConvertToTyped(obj, svc); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, svc, nil)
}

// reconcileRedisHAAnnounceService reconciles a per-replica announce Service for Redis HA
func (r *RedisController) reconcileRedisHAAnnounceService(ctx context.Context, cr *argoproj.ArgoCD, index int32) error {
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithExtra("AnnounceIndex", index)

	obj, err := r.templateEngine.RenderManifest("service-ha-announce.yaml.tmpl", data)
	if err != nil {
		return err
	}

	svc := &corev1.Service{}
	if err := template.ConvertToTyped(obj, svc); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, svc, nil)
}

// reconcileRedisHAProxyService reconciles the HAProxy Service for Redis HA
func (r *RedisController) reconcileRedisHAProxyService(ctx context.Context, cr *argoproj.ArgoCD) error {
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace))

	obj, err := r.templateEngine.RenderManifest("service-ha-haproxy.yaml.tmpl", data)
	if err != nil {
		return err
	}

	svc := &corev1.Service{}
	if err := template.ConvertToTyped(obj, svc); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, svc, nil)
}

// reconcileRedisHAProxyDeployment reconciles the HAProxy Deployment for Redis HA
func (r *RedisController) reconcileRedisHAProxyDeployment(ctx context.Context, cr *argoproj.ArgoCD) error {
	log := r.logWithValues(cr)
	log.Info("reconciling redis HA proxy deployment")

	useTLS := redisShouldUseTLS(r.Client, cr)

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis-ha").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(common.DefaultAnnotations(cr.Name, cr.Namespace)).
		WithServiceAccount(cr.Name + "-redis-ha").
		WithExtra("ImagePullPolicy", string(argoutil.GetImagePullPolicy(cr.Spec.ImagePullPolicy))).
		WithExtra("HAProxyImage", getRedisHAProxyContainerImage(cr)).
		WithExtra("UseTLS", useTLS).
		WithExtra("TLSSecretName", common.ArgoCDRedisServerTLSSecretName).
		WithExtra("Replicas", getRedisHAReplicas())

	resources := getRedisHAResources(cr)
	if resources.Limits != nil || resources.Requests != nil {
		data.WithExtra("Resources", component.ResourcesToTemplate(resources))
	}

	component.ApplyNodePlacement(data, cr.Spec.NodePlacement)

	obj, err := r.templateEngine.RenderManifest("deployment-haproxy.yaml.tmpl", data)
	if err != nil {
		return err
	}

	deploy := &appsv1.Deployment{}
	if err := template.ConvertToTyped(obj, deploy); err != nil {
		return err
	}

	return component.ReconcileResource(ctx, r.Client, r.Scheme, cr, deploy, r.Decorators)
}
