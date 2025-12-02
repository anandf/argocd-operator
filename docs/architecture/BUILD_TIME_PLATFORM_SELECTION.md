# Build-Time Platform Selection

## Overview

This document describes the shift from **runtime platform detection** to **build-time platform selection** combined with **runtime API detection**.

## Motivation

The previous approach used runtime platform detection to determine if the operator was running on Kubernetes or OpenShift. This had several drawbacks:

1. **Uncertainty**: The platform wasn't known until runtime
2. **Tight coupling**: All platform-specific code was compiled into every binary
3. **Testing complexity**: Harder to test platform-specific behavior
4. **Binary size**: Larger binaries with unused code for the other platform

## New Approach

### Build-Time Platform Selection

The platform is now determined at **build time** using Go build tags:

```bash
# Kubernetes platform (default)
go build

# OpenShift platform
go build -tags openshift
```

This approach provides:
- **Certainty**: Platform is known at compile time
- **Smaller binaries**: Only relevant platform code is compiled
- **Better testing**: Can build and test platform-specific binaries separately
- **Clearer deployment**: Explicit platform-specific images

### Runtime API Detection

While the platform is fixed at build time, the operator still needs runtime flexibility for specific APIs:

#### Use Case: Route vs Ingress

The server component needs to expose the Argo CD UI. On OpenShift, this should use a Route. On Kubernetes, it should use an Ingress.

**Old approach** (problematic):
```go
if isOpenShift {
    createRoute()
} else {
    createIngress()
}
```

**New approach** (correct):
```go
// At build time: OpenShift binary includes Route support
// At runtime: Check if Route API is available
if apiDetector.HasRoute(ctx) {
    createRoute()
} else if apiDetector.HasIngress(ctx) {
    createIngress()
} else {
    return error("no suitable API for exposing server")
}
```

This handles cases like:
- OpenShift cluster without Route API (misconfigured)
- Kubernetes cluster with Route API installed
- Future APIs like Gateway API

## Implementation

### Build Tags

Two platform files with build tags:

**pkg/platform/platform_kubernetes.go**
```go
//go:build !openshift
// +build !openshift

package platform

func NewPlatform(c client.Client, scheme *runtime.Scheme) Platform {
    // Kubernetes-specific setup
    // No SCC decorator, etc.
}
```

**pkg/platform/platform_openshift.go**
```go
//go:build openshift
// +build openshift

package platform

func NewPlatform(c client.Client, scheme *runtime.Scheme) Platform {
    // OpenShift-specific setup
    // Includes SCC decorator, etc.
}
```

### API Detector

**pkg/platform/api_detector.go**
```go
type APIDetector struct {
    client          client.Client
    discoveryClient discovery.DiscoveryInterface
    cache           map[schema.GroupVersionResource]bool
}

func (d *APIDetector) HasAPI(ctx context.Context, gvr schema.GroupVersionResource) bool {
    // Check if API is available using discovery client
}

func (d *APIDetector) HasRoute(ctx context.Context) bool {
    return d.HasAPI(ctx, RouteGVR)
}

func (d *APIDetector) HasIngress(ctx context.Context) bool {
    return d.HasAPI(ctx, IngressGVR)
}
```

### Controller Updates

All component controllers now accept an `APIDetector`:

```go
type Controller interface {
    Reconcile(cr *argoproj.ArgoCD, apiDetector *APIDetector) error
}
```

Example usage in ServerController:

```go
func (r *ServerController) reconcileServerIngress(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error {
    ctx := context.Background()

    // Check what's actually available
    if apiDetector.HasRoute(ctx) {
        return r.createRoute(cr)
    }

    if apiDetector.HasIngress(ctx) {
        return r.createIngress(cr)
    }

    if apiDetector.HasGateway(ctx) {
        return r.createGateway(cr)
    }

    return fmt.Errorf("no suitable API found for exposing server")
}
```

## Building

### Makefile Targets

```bash
# Build for Kubernetes (default)
make build
make build-kubernetes

# Build for OpenShift
make build-openshift

# Build both
make build-all-platforms

# Run locally
make run-kubernetes
make run-openshift

# Docker images
make docker-build-kubernetes
make docker-build-openshift
```

### Dockerfile

Updated to accept BUILD_TAGS argument:

```dockerfile
ARG BUILD_TAGS=""
RUN go build -tags="$BUILD_TAGS" -o manager cmd/main.go
```

## Benefits

### 1. Explicit Platform Selection

No more surprises about which platform the operator thinks it's running on.

### 2. Smaller Binaries

Kubernetes binary doesn't include OpenShift-specific decorators like SCC.

### 3. Better Error Messages

```
# Old approach
ERROR: Route API not found (is this OpenShift?)

# New approach
INFO: Route API not available, using Ingress instead
# or
ERROR: Server ingress/route enabled but no suitable API found
  (routeEnabled=true, ingressEnabled=true, hasRoute=false, hasIngress=false)
```

### 4. Future-Proof

Easy to add new platform targets or API alternatives:

```bash
# Future: Build for AWS (with AWS Load Balancer Controller)
go build -tags aws

# Future: Support Gateway API
if apiDetector.HasGateway(ctx) {
    createGateway()
}
```

### 5. Testing

Can test platform-specific behavior in isolation:

```bash
# Test Kubernetes-specific behavior
go test -tags '' ./pkg/platform

# Test OpenShift-specific behavior
go test -tags openshift ./pkg/platform
```

## Migration Path

### For Developers

1. Always use `make build-kubernetes` or `make build-openshift` explicitly
2. When adding new platform-specific code, use build tags
3. When checking for API availability, use `apiDetector.HasAPI()`
4. Don't assume platform based on API availability

### For Users/Operators

1. Download the correct binary for your platform:
   - `argocd-operator-kubernetes:v0.17.0` for Kubernetes
   - `argocd-operator-openshift:v0.17.0` for OpenShift

2. Use platform-specific images:
   ```yaml
   # Kubernetes
   image: quay.io/argoprojlabs/argocd-operator:v0.17.0-kubernetes

   # OpenShift
   image: quay.io/argoprojlabs/argocd-operator:v0.17.0-openshift
   ```

## Common Patterns

### Adding a New Platform-Specific Decorator

**Step 1:** Create the decorator
```bash
# pkg/decorator/mydecorator.go
```

**Step 2:** Add to platform implementation
```go
// pkg/platform/platform_openshift.go
//go:build openshift

func NewPlatform(...) Platform {
    p.decorators["mydec"] = decorator.NewMyDecorator(c, scheme)
}
```

**Step 3:** Build and test
```bash
make build-openshift
```

### Adding Runtime API Detection

**Step 1:** Define the GVR
```go
// pkg/platform/api_detector.go
var MyAPIGVR = schema.GroupVersionResource{
    Group:    "example.com",
    Version:  "v1",
    Resource: "myresources",
}
```

**Step 2:** Add helper method
```go
func (d *APIDetector) HasMyAPI(ctx context.Context) bool {
    return d.HasAPI(ctx, MyAPIGVR)
}
```

**Step 3:** Use in controller
```go
func (r *MyController) Reconcile(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error {
    if apiDetector.HasMyAPI(ctx) {
        // Create MyResource
    }
}
```

## FAQs

### Q: Can I still build a "universal" binary?

**A:** No, that's the point. Each binary is platform-specific. Deploy the right binary for your platform.

### Q: What if my Kubernetes cluster has the Route API installed?

**A:** The API detector will find it and use it if the ArgoCD CR enables routes. The operator adapts to what's available.

### Q: What happens if I deploy the OpenShift binary to Kubernetes?

**A:** It will try to apply OpenShift-specific decorators (like SCC) which might fail or be ignored depending on your cluster's RBAC setup. Use the correct binary.

### Q: Can I still test both platforms locally?

**A:** Yes! Build both binaries and run them against different clusters:
```bash
make build-kubernetes
./bin/manager-kubernetes  # test against k8s cluster

make build-openshift
./bin/manager-openshift   # test against openshift cluster
```

### Q: Is runtime platform detection completely removed?

**A:** The old `DetectPlatform()` function is deprecated but kept for backward compatibility. It will be removed in a future release. Use `NewPlatform()` which is build-tag selected.

## References

- [Go Build Tags Documentation](https://pkg.go.dev/cmd/go#hdr-Build_constraints)
- [Kubernetes Discovery Client](https://pkg.go.dev/k8s.io/client-go/discovery)
- [Build Tags Best Practices](https://www.digitalocean.com/community/tutorials/customizing-go-binaries-with-build-tags)
