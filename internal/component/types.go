package component

import (
	"context"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Component defines the interface that all component controllers must implement.
// Each component has explicit lifecycle methods for applying and removing resources,
// along with introspection methods for name and enabled state.
type Component interface {
	// Name returns the component's name (e.g., "redis", "server", "dex").
	Name() string

	// IsEnabled returns whether this component should be active for the given ArgoCD CR.
	IsEnabled(cr *argoproj.ArgoCD) bool

	// Ensure creates or updates all resources for this component.
	Ensure(ctx context.Context, cr *argoproj.ArgoCD) error

	// Remove deletes all resources managed by this component.
	Remove(ctx context.Context, cr *argoproj.ArgoCD) error
}

// CapabilityCheck determines whether a particular API or capability is available.
// It receives a Kubernetes client so implementations can perform runtime discovery
// (e.g., checking whether a CRD exists). For build-time known capabilities, the
// client can be ignored.
type CapabilityCheck func(c client.Client) bool

// Well-known capability names for API availability checks.
const (
	RouteAPI          = "route"
	IngressAPI        = "ingress"
	ServiceMonitorAPI = "servicemonitor"
)

// ComponentConfig holds platform-level capabilities registered via functional options.
// Components query capabilities with IsAvailable instead of reading struct fields,
// making it easy to add new capability checks without modifying this struct.
type ComponentConfig struct {
	capabilities map[string]CapabilityCheck
}

// IsAvailable returns true if the named capability is registered and its check
// passes. The client is forwarded to the CapabilityCheck so it can perform
// runtime discovery when needed.
func (c *ComponentConfig) IsAvailable(name string, cl client.Client) bool {
	check, ok := c.capabilities[name]
	if !ok {
		return false
	}
	return check(cl)
}

// ComponentOption is a functional option for configuring ComponentConfig.
type ComponentOption func(*ComponentConfig)

// WithCapability registers a named capability with a custom check function.
func WithCapability(name string, check CapabilityCheck) ComponentOption {
	return func(c *ComponentConfig) {
		c.capabilities[name] = check
	}
}

// WithRoute registers the Route API as available (build-time known).
func WithRoute() ComponentOption {
	return WithCapability(RouteAPI, func(_ client.Client) bool { return true })
}

// WithIngress registers the Ingress API as available (build-time known).
func WithIngress() ComponentOption {
	return WithCapability(IngressAPI, func(_ client.Client) bool { return true })
}

// WithServiceMonitor registers the ServiceMonitor API as available (build-time known).
func WithServiceMonitor() ComponentOption {
	return WithCapability(ServiceMonitorAPI, func(_ client.Client) bool { return true })
}

// NewComponentConfig creates a ComponentConfig by applying the given options.
func NewComponentConfig(opts ...ComponentOption) *ComponentConfig {
	cfg := &ComponentConfig{
		capabilities: make(map[string]CapabilityCheck),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
