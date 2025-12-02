package platform

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

var apiDetectorLog logr.Logger = logs.Log.WithName("api-detector")

// APIDetector provides runtime detection of available Kubernetes APIs
type APIDetector struct {
	client          client.Client
	discoveryClient discovery.DiscoveryInterface
	cache           map[schema.GroupVersionResource]bool
	cacheMu         sync.RWMutex
}

// NewAPIDetector creates a new API detector
func NewAPIDetector(c client.Client, config *rest.Config) (*APIDetector, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	return &APIDetector{
		client:          c,
		discoveryClient: discoveryClient,
		cache:           make(map[schema.GroupVersionResource]bool),
	}, nil
}

// HasAPI checks if a specific API resource is available in the cluster
// This is the primary method for runtime API detection
func (d *APIDetector) HasAPI(ctx context.Context, gvr schema.GroupVersionResource) bool {
	// Check cache first
	d.cacheMu.RLock()
	if result, found := d.cache[gvr]; found {
		d.cacheMu.RUnlock()
		return result
	}
	d.cacheMu.RUnlock()

	// Not in cache, perform discovery
	exists := d.checkAPIExists(ctx, gvr)

	// Cache the result
	d.cacheMu.Lock()
	d.cache[gvr] = exists
	d.cacheMu.Unlock()

	return exists
}

// checkAPIExists performs the actual API existence check
func (d *APIDetector) checkAPIExists(ctx context.Context, gvr schema.GroupVersionResource) bool {
	// Use discovery client to check if the API resource exists
	resourceList, err := d.discoveryClient.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		if errors.IsNotFound(err) {
			apiDetectorLog.V(1).Info("API group version not found", "gv", gvr.GroupVersion().String())
			return false
		}
		// Other errors might be transient, log but assume API doesn't exist
		apiDetectorLog.Error(err, "error checking API availability", "gvr", gvr.String())
		return false
	}

	// Check if the specific resource exists in the group version
	for _, resource := range resourceList.APIResources {
		if resource.Name == gvr.Resource {
			apiDetectorLog.V(1).Info("API resource found", "gvr", gvr.String())
			return true
		}
	}

	apiDetectorLog.V(1).Info("API resource not found in group version", "gvr", gvr.String())
	return false
}

// InvalidateCache clears the API cache, forcing re-detection
func (d *APIDetector) InvalidateCache() {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.cache = make(map[schema.GroupVersionResource]bool)
	apiDetectorLog.Info("API cache invalidated")
}

// PreferredAPIForResource determines which API to use based on availability
// For example: Route (OpenShift) > Ingress (Kubernetes) > Gateway (Gateway API)
func (d *APIDetector) PreferredAPIForResource(ctx context.Context, resourceType string) (schema.GroupVersionResource, bool) {
	var candidates []schema.GroupVersionResource

	switch resourceType {
	case "route-or-ingress":
		// Check for Route first (OpenShift), then Ingress (standard Kubernetes)
		candidates = []schema.GroupVersionResource{
			{Group: "route.openshift.io", Version: "v1", Resource: "routes"},
			{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		}
	case "gateway":
		// Check for Gateway API
		candidates = []schema.GroupVersionResource{
			{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"},
			{Group: "gateway.networking.k8s.io", Version: "v1beta1", Resource: "gateways"},
		}
	case "servicemonitor":
		// Check for Prometheus ServiceMonitor
		candidates = []schema.GroupVersionResource{
			{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"},
		}
	default:
		apiDetectorLog.Info("unknown resource type requested", "type", resourceType)
		return schema.GroupVersionResource{}, false
	}

	// Return the first available API
	for _, gvr := range candidates {
		if d.HasAPI(ctx, gvr) {
			apiDetectorLog.Info("selected preferred API", "resourceType", resourceType, "gvr", gvr.String())
			return gvr, true
		}
	}

	apiDetectorLog.Info("no available API found for resource type", "type", resourceType)
	return schema.GroupVersionResource{}, false
}

// GetRESTMapping returns the REST mapping for a GVR
func (d *APIDetector) GetRESTMapping(gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	mapper := d.client.RESTMapper()
	return mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
}

// Common API resource definitions for convenience
var (
	RouteGVR = schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}

	IngressGVR = schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "ingresses",
	}

	GatewayGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}

	ServiceMonitorGVR = schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "servicemonitors",
	}

	ClusterVersionGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
)

// HasRoute checks if the Route API (OpenShift) is available
func (d *APIDetector) HasRoute(ctx context.Context) bool {
	return d.HasAPI(ctx, RouteGVR)
}

// HasIngress checks if the Ingress API (standard Kubernetes) is available
func (d *APIDetector) HasIngress(ctx context.Context) bool {
	return d.HasAPI(ctx, IngressGVR)
}

// HasGateway checks if the Gateway API is available
func (d *APIDetector) HasGateway(ctx context.Context) bool {
	return d.HasAPI(ctx, GatewayGVR)
}

// HasServiceMonitor checks if the ServiceMonitor API (Prometheus Operator) is available
func (d *APIDetector) HasServiceMonitor(ctx context.Context) bool {
	return d.HasAPI(ctx, ServiceMonitorGVR)
}

// HasClusterVersion checks if the ClusterVersion API (OpenShift) is available
func (d *APIDetector) HasClusterVersion(ctx context.Context) bool {
	return d.HasAPI(ctx, ClusterVersionGVR)
}

// ListAPIResources lists all available API resources for debugging
func (d *APIDetector) ListAPIResources(ctx context.Context) ([]metav1.APIResourceList, error) {
	_, resourceLists, err := d.discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to list API resources: %w", err)
	}
	return resourceLists, nil
}
