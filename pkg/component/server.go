package component

import (
	"context"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/pkg/platform"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

// ServerController manages the Argo CD Server component
type ServerController struct {
	Client client.Client
	Scheme *runtime.Scheme
	logger logr.Logger
}

func NewServerController(client client.Client, scheme *runtime.Scheme) *ServerController {
	return &ServerController{
		Client: client,
		Scheme: scheme,
		logger: logs.Log.WithName("ServerController"),
	}
}

func (r *ServerController) Reconcile(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error {
	r.logger.Info("reconciling server component")

	// Reconcile server service account
	r.logger.Info("reconciling server serviceaccount")
	sa, err := r.reconcileServerServiceAccount(cr)
	if err != nil {
		return err
	}

	// Reconcile server role
	r.logger.Info("reconciling server role")
	role, err := r.reconcileServerRole(cr)
	if err != nil {
		return err
	}

	// Reconcile server role binding
	r.logger.Info("reconciling server role binding")
	if err := r.reconcileServerRoleBinding(cr, role, sa); err != nil {
		return err
	}

	// Reconcile server deployment
	r.logger.Info("reconciling server deployment")
	if err := r.reconcileServerDeployment(cr, sa); err != nil {
		return err
	}

	// Reconcile server service
	r.logger.Info("reconciling server service")
	if err := r.reconcileServerService(cr); err != nil {
		return err
	}

	// Reconcile server metrics service
	r.logger.Info("reconciling server metrics service")
	if err := r.reconcileServerMetricsService(cr); err != nil {
		return err
	}

	// Reconcile ingress/route based on API availability
	r.logger.Info("reconciling server ingress/route")
	if err := r.reconcileServerIngress(cr, apiDetector); err != nil {
		return err
	}

	r.logger.Info("server component reconciliation complete")
	return nil
}

// reconcileServerServiceAccount reconciles the ServiceAccount for the Server component
func (r *ServerController) reconcileServerServiceAccount(cr *argoproj.ArgoCD) (interface{}, error) {
	// TODO: Implement server service account reconciliation
	// This will be extracted from controllers/argocd/service_account.go
	r.logger.Info("server service account reconciliation - placeholder")
	return nil, nil
}

// reconcileServerRole reconciles the Role for the Server component
func (r *ServerController) reconcileServerRole(cr *argoproj.ArgoCD) (interface{}, error) {
	// TODO: Implement server role reconciliation
	// This will be extracted from controllers/argocd/role.go
	r.logger.Info("server role reconciliation - placeholder")
	return nil, nil
}

// reconcileServerRoleBinding reconciles the RoleBinding for the Server component
func (r *ServerController) reconcileServerRoleBinding(cr *argoproj.ArgoCD, role, sa interface{}) error {
	// TODO: Implement server role binding reconciliation
	// This will be extracted from controllers/argocd/rolebinding.go
	r.logger.Info("server role binding reconciliation - placeholder")
	return nil
}

// reconcileServerDeployment reconciles the Deployment for the Server component
func (r *ServerController) reconcileServerDeployment(cr *argoproj.ArgoCD, sa interface{}) error {
	// TODO: Implement server deployment reconciliation
	// This will be extracted from controllers/argocd/deployment.go
	r.logger.Info("server deployment reconciliation - placeholder")
	return nil
}

// reconcileServerService reconciles the Service for the Server component
func (r *ServerController) reconcileServerService(cr *argoproj.ArgoCD) error {
	// TODO: Implement server service reconciliation
	// This will be extracted from controllers/argocd/service.go
	r.logger.Info("server service reconciliation - placeholder")
	return nil
}

// reconcileServerMetricsService reconciles the metrics Service for the Server component
func (r *ServerController) reconcileServerMetricsService(cr *argoproj.ArgoCD) error {
	// TODO: Implement server metrics service reconciliation
	// This will be extracted from controllers/argocd/service.go
	r.logger.Info("server metrics service reconciliation - placeholder")
	return nil
}

// reconcileServerIngress reconciles the Ingress or Route for the Server component
// based on what APIs are available in the cluster
func (r *ServerController) reconcileServerIngress(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error {
	ctx := context.Background()

	// Check if server ingress/route is enabled in the CR
	if !cr.Spec.Server.Ingress.Enabled && !cr.Spec.Server.Route.Enabled {
		r.logger.Info("server ingress/route not enabled, skipping")
		return nil
	}

	// Determine which API to use based on availability
	// Priority: Route (if available and enabled) > Ingress (if available and enabled) > Gateway (future)
	if cr.Spec.Server.Route.Enabled && apiDetector.HasRoute(ctx) {
		r.logger.Info("creating server route (OpenShift)")
		// TODO: Implement route creation
		// This will be extracted from controllers/argocd/route.go
		return nil
	}

	if cr.Spec.Server.Ingress.Enabled && apiDetector.HasIngress(ctx) {
		r.logger.Info("creating server ingress (Kubernetes)")
		// TODO: Implement ingress creation
		// This will be extracted from controllers/argocd/ingress.go
		return nil
	}

	// Check for Gateway API as a fallback
	if apiDetector.HasGateway(ctx) {
		r.logger.Info("gateway API available but not yet implemented")
		// TODO: Implement Gateway API support in future
		return nil
	}

	// No suitable API found
	if cr.Spec.Server.Route.Enabled || cr.Spec.Server.Ingress.Enabled {
		r.logger.Info("ingress/route enabled but no suitable API found in cluster",
			"routeEnabled", cr.Spec.Server.Route.Enabled,
			"ingressEnabled", cr.Spec.Server.Ingress.Enabled,
			"hasRoute", apiDetector.HasRoute(ctx),
			"hasIngress", apiDetector.HasIngress(ctx))
	}

	return nil
}
