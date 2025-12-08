# Proposal: Shared Spec and Reconciliation Logic

## Summary

Refactor ArgoCD and ClusterArgoCD to use shared spec structs and shared reconciliation logic to eliminate code duplication and improve maintainability.

## Problem Statement

Currently:
1. **ArgoCDSpec** and **ClusterArgoCDSpec** have ~95% identical fields (duplicated ~200 lines of code)
2. **ArgoCD controller** and **ClusterArgoCD controller** will have similar reconciliation logic (duplicated ~2000 lines of code)
3. Any changes to common fields must be made in both places
4. Increased maintenance burden and risk of inconsistencies

## Proposed Solution

### Part 1: Shared Spec Using Struct Embedding

#### 1.1 Create ArgoCDCommonSpec (DONE ✅)

```go
// File: api/v1beta1/argocd_common_types.go
type ArgoCDCommonSpec struct {
    ApplicationSet *ArgoCDApplicationSet
    Controller ArgoCDApplicationControllerSpec
    Server ArgoCDServerSpec
    Repo ArgoCDRepoSpec
    Redis ArgoCDRedisSpec
    // ... all ~40 common fields
}
```

#### 1.2 Refactor ClusterArgoCDSpec to Embed Common Spec

**Before:**
```go
type ClusterArgoCDSpec struct {
    ApplicationSet *ArgoCDApplicationSet  // Duplicated
    Controller ArgoCDApplicationControllerSpec  // Duplicated
    Server ArgoCDServerSpec  // Duplicated
    // ... 40 more duplicated fields ...

    // ClusterArgoCD-specific fields
    ControlPlaneNamespace string
    SourceNamespaces []string
    ArgoCDAgent *ArgoCDAgentSpec
}
```

**After:**
```go
type ClusterArgoCDSpec struct {
    // Embed all common fields
    ArgoCDCommonSpec `json:",inline"`

    // ClusterArgoCD-specific fields only
    // ControlPlaneNamespace where ClusterArgoCD control plane components will be deployed
    // This includes namespace-scoped resources like Deployments, StatefulSets, ConfigMaps, Services, etc.
    ControlPlaneNamespace string `json:"controlPlaneNamespace,omitempty"`

    // SourceNamespaces for cross-namespace Application management
    SourceNamespaces []string `json:"sourceNamespaces,omitempty"`

    // ArgoCDAgent configuration for cluster-scoped instances
    ArgoCDAgent *ArgoCDAgentSpec `json:"argoCDAgent,omitempty"`
}
```

**Benefits:**
- Reduces ClusterArgoCDSpec from ~250 lines to ~20 lines
- Single source of truth for common fields
- Automatic propagation of changes

#### 1.3 Refactor ArgoCDSpec to Embed Common Spec

**Before:**
```go
type ArgoCDSpec struct {
    ApplicationSet *ArgoCDApplicationSet  // Duplicated
    Controller ArgoCDApplicationControllerSpec  // Duplicated
    // ... 40 more duplicated fields ...

    // Deprecated fields for namespace-scoped instances
    SourceNamespaces []string  // Deprecated
    ArgoCDAgent *ArgoCDAgentSpec  // Deprecated
}
```

**After:**
```go
type ArgoCDSpec struct {
    // Embed all common fields
    ArgoCDCommonSpec `json:",inline"`

    // Deprecated fields (kept for backwards compatibility)
    // Deprecated: Use ClusterArgoCD for cross-namespace management
    SourceNamespaces []string `json:"sourceNamespaces,omitempty"`

    // Deprecated: Use ClusterArgoCD for ArgoCD Agent
    ArgoCDAgent *ArgoCDAgentSpec `json:"argoCDAgent,omitempty"`
}
```

**Benefits:**
- Reduces ArgoCDSpec from ~250 lines to ~15 lines
- Clear separation of deprecated fields
- Same struct access pattern (backward compatible)

### Part 2: Shared Reconciliation Logic

#### 2.1 Create Shared Reconciliation Package

```
controllers/
├── argocd/
│   └── argocd_controller.go         (uses shared logic)
├── clusterargocd/
│   └── clusterargocd_controller.go  (uses shared logic)
└── shared/
    ├── reconciler.go                (shared reconciliation logic)
    ├── deployment.go                (shared deployment logic)
    ├── service.go                   (shared service logic)
    ├── statefulset.go               (shared statefulset logic)
    ├── serviceaccount.go            (shared serviceaccount logic)
    └── types.go                     (shared interfaces)
```

#### 2.2 Define ArgoCD Instance Interface

```go
// File: controllers/shared/types.go
package shared

import (
    argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArgoCDInstance is an interface that both ArgoCD and ClusterArgoCD implement
// This allows shared reconciliation logic to work with both types
type ArgoCDInstance interface {
    // GetName returns the name of the ArgoCD instance
    GetName() string

    // GetNamespace returns the namespace for namespace-scoped instances
    // or the target deployment namespace for cluster-scoped instances
    GetNamespace() string

    // GetCommonSpec returns the common spec shared by both types
    GetCommonSpec() *argoproj.ArgoCDCommonSpec

    // GetStatus returns the status
    GetStatus() *argoproj.ArgoCDStatus

    // IsClusterScoped returns true if this is a ClusterArgoCD instance
    IsClusterScoped() bool

    // GetSourceNamespaces returns source namespaces for cross-namespace management
    GetSourceNamespaces() []string

    // GetObjectMeta returns the object metadata
    GetObjectMeta() *metav1.ObjectMeta

    // GetApplicationSet returns ApplicationSet configuration
    GetApplicationSet() *argoproj.ArgoCDApplicationSet

    // GetNotifications returns Notifications configuration
    GetNotifications() argoproj.ArgoCDNotifications

    // GetArgoCDAgent returns Agent configuration (nil for namespace-scoped)
    GetArgoCDAgent() *argoproj.ArgoCDAgentSpec
}
```

#### 2.3 Implement Interface for ArgoCD

```go
// File: api/v1beta1/argocd_interface.go
package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Ensure ArgoCD implements ArgoCDInstance interface
var _ ArgoCDInstance = &ArgoCD{}

func (a *ArgoCD) GetName() string {
    return a.Name
}

func (a *ArgoCD) GetNamespace() string {
    return a.Namespace
}

func (a *ArgoCD) GetCommonSpec() *ArgoCDCommonSpec {
    return &a.Spec.ArgoCDCommonSpec
}

func (a *ArgoCD) GetStatus() *ArgoCDStatus {
    return &a.Status
}

func (a *ArgoCD) IsClusterScoped() bool {
    return false
}

func (a *ArgoCD) GetSourceNamespaces() []string {
    // For namespace-scoped, return empty (deprecated field)
    return nil
}

func (a *ArgoCD) GetObjectMeta() *metav1.ObjectMeta {
    return &a.ObjectMeta
}

func (a *ArgoCD) GetApplicationSet() *ArgoCDApplicationSet {
    return a.Spec.ApplicationSet
}

func (a *ArgoCD) GetNotifications() ArgoCDNotifications {
    return a.Spec.Notifications
}

func (a *ArgoCD) GetArgoCDAgent() *ArgoCDAgentSpec {
    return nil // Not supported for namespace-scoped
}
```

#### 2.4 Implement Interface for ClusterArgoCD

```go
// File: api/v1beta1/clusterargocd_interface.go
package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Ensure ClusterArgoCD implements ArgoCDInstance interface
var _ ArgoCDInstance = &ClusterArgoCD{}

func (c *ClusterArgoCD) GetName() string {
    return c.Name
}

func (c *ClusterArgoCD) GetNamespace() string {
    // For cluster-scoped, return the target deployment namespace
    if c.Spec.Namespace != "" {
        return c.Spec.Namespace
    }
    return "argocd" // default
}

func (c *ClusterArgoCD) GetCommonSpec() *ArgoCDCommonSpec {
    return &c.Spec.ArgoCDCommonSpec
}

func (c *ClusterArgoCD) GetStatus() *ArgoCDStatus {
    return &c.Status
}

func (c *ClusterArgoCD) IsClusterScoped() bool {
    return true
}

func (c *ClusterArgoCD) GetSourceNamespaces() []string {
    return c.Spec.SourceNamespaces
}

func (c *ClusterArgoCD) GetObjectMeta() *metav1.ObjectMeta {
    return &c.ObjectMeta
}

func (c *ClusterArgoCD) GetApplicationSet() *ArgoCDApplicationSet {
    return c.Spec.ApplicationSet
}

func (c *ClusterArgoCD) GetNotifications() ArgoCDNotifications {
    return c.Spec.Notifications
}

func (c *ClusterArgoCD) GetArgoCDAgent() *ArgoCDAgentSpec {
    return c.Spec.ArgoCDAgent
}
```

#### 2.5 Shared Reconciliation Functions

```go
// File: controllers/shared/reconciler.go
package shared

import (
    "context"

    argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerConfig holds configuration for shared reconciliation
type ReconcilerConfig struct {
    Client    client.Client
    K8sClient kubernetes.Interface
}

// ReconcileServiceAccounts reconciles ServiceAccounts for any ArgoCD instance
func ReconcileServiceAccounts(ctx context.Context, instance ArgoCDInstance, config *ReconcilerConfig) error {
    namespace := instance.GetNamespace()
    isClusterScoped := instance.IsClusterScoped()

    // Reconcile application-controller ServiceAccount
    if err := reconcileServiceAccount(
        ctx,
        instance,
        "application-controller",
        namespace,
        isClusterScoped,
        config,
    ); err != nil {
        return err
    }

    // Reconcile server ServiceAccount
    if err := reconcileServiceAccount(
        ctx,
        instance,
        "server",
        namespace,
        isClusterScoped,
        config,
    ); err != nil {
        return err
    }

    // More components...
    return nil
}

// ReconcileDeployments reconciles Deployments for any ArgoCD instance
func ReconcileDeployments(ctx context.Context, instance ArgoCDInstance, config *ReconcilerConfig) error {
    namespace := instance.GetNamespace()
    commonSpec := instance.GetCommonSpec()

    // Reconcile server deployment
    if err := reconcileServerDeployment(
        ctx,
        instance,
        namespace,
        commonSpec.Server,
        config,
    ); err != nil {
        return err
    }

    // Reconcile repo-server deployment
    if err := reconcileRepoServerDeployment(
        ctx,
        instance,
        namespace,
        commonSpec.Repo,
        config,
    ); err != nil {
        return err
    }

    // More deployments...
    return nil
}

// ReconcileServices reconciles Services for any ArgoCD instance
func ReconcileServices(ctx context.Context, instance ArgoCDInstance, config *ReconcilerConfig) error {
    // Shared service reconciliation logic
    return nil
}

// ReconcileStatefulSets reconciles StatefulSets for any ArgoCD instance
func ReconcileStatefulSets(ctx context.Context, instance ArgoCDInstance, config *ReconcilerConfig) error {
    // Shared statefulset reconciliation logic
    return nil
}
```

#### 2.6 Refactored Controller Usage

**ArgoCD Controller:**
```go
// File: controllers/argocd/argocd_controller.go
func (r *ReconcileArgoCD) reconcileResources(cr *argoproj.ArgoCD, status *argoproj.ArgoCDStatus) error {
    config := &shared.ReconcilerConfig{
        Client:    r.Client,
        K8sClient: r.K8sClient,
    }

    // Use shared reconciliation logic
    if err := shared.ReconcileServiceAccounts(context.TODO(), cr, config); err != nil {
        return err
    }

    if err := shared.ReconcileDeployments(context.TODO(), cr, config); err != nil {
        return err
    }

    if err := shared.ReconcileServices(context.TODO(), cr, config); err != nil {
        return err
    }

    // ArgoCD-specific logic (namespace-scoped RBAC)
    if err := r.reconcileRoles(cr); err != nil {
        return err
    }

    return nil
}
```

**ClusterArgoCD Controller:**
```go
// File: controllers/clusterargocd/clusterargocd_controller.go
func (r *ReconcileClusterArgoCD) reconcileResources(cr *argoproj.ClusterArgoCD, status *argoproj.ArgoCDStatus) error {
    config := &shared.ReconcilerConfig{
        Client:    r.Client,
        K8sClient: r.K8sClient,
    }

    // Use shared reconciliation logic
    if err := shared.ReconcileServiceAccounts(context.TODO(), cr, config); err != nil {
        return err
    }

    if err := shared.ReconcileDeployments(context.TODO(), cr, config); err != nil {
        return err
    }

    if err := shared.ReconcileServices(context.TODO(), cr, config); err != nil {
        return err
    }

    // ClusterArgoCD-specific logic (cluster-scoped RBAC)
    if err := r.reconcileClusterRoles(cr); err != nil {
        return err
    }

    if err := r.reconcileSourceNamespaces(cr); err != nil {
        return err
    }

    return nil
}
```

## Benefits

### Code Reduction
- **ArgoCDSpec**: ~250 lines → ~15 lines (94% reduction)
- **ClusterArgoCDSpec**: ~250 lines → ~20 lines (92% reduction)
- **Reconciliation Logic**: ~2000 duplicated lines → ~500 shared lines (75% reduction)

### Maintainability
- Single source of truth for common fields
- Changes automatically apply to both ArgoCD and ClusterArgoCD
- Clear separation of scope-specific logic

### Type Safety
- Interface enforcement ensures both types implement required methods
- Compile-time verification of compatibility

### Testing
- Shared logic can be tested once
- Interface mocking for unit tests
- Reduced test duplication

## Implementation Plan

### Phase 1: API Refactoring (PARTIALLY DONE)
- [x] Create `ArgoCDCommonSpec` (DONE)
- [ ] Add interface definition to `api/v1beta1/`
- [ ] Refactor `ClusterArgoCDSpec` to embed `ArgoCDCommonSpec`
- [ ] Refactor `ArgoCDSpec` to embed `ArgoCDCommonSpec`
- [ ] Implement interface methods for `ArgoCD`
- [ ] Implement interface methods for `ClusterArgoCD`
- [ ] Run `make manifests generate`
- [ ] Verify CRD generation (should be identical)

### Phase 2: Shared Reconciliation Package
- [ ] Create `controllers/shared/` package
- [ ] Define `ArgoCDInstance` interface
- [ ] Implement shared ServiceAccount reconciliation
- [ ] Implement shared Deployment reconciliation
- [ ] Implement shared Service reconciliation
- [ ] Implement shared StatefulSet reconciliation
- [ ] Add utility functions for common operations

### Phase 3: Controller Refactoring
- [ ] Refactor `ArgoCD` controller to use shared logic
- [ ] Refactor `ClusterArgoCD` controller to use shared logic
- [ ] Keep scope-specific logic in respective controllers
- [ ] Update tests

### Phase 4: Testing & Validation
- [ ] Unit tests for shared package
- [ ] Integration tests for both controller types
- [ ] Verify backward compatibility
- [ ] E2E tests

## Backward Compatibility

### CRD Compatibility
The embedded struct approach maintains JSON compatibility:
- Field paths remain the same: `spec.server.image`
- CRD manifests are identical
- Existing resources continue to work

### Controller Compatibility
- Both controllers continue to work independently
- No changes to reconciliation behavior
- Gradual migration possible

## Migration Path

1. **Phase 1**: Implement common spec (no behavioral changes)
2. **Phase 2**: Add shared reconciliation package alongside existing logic
3. **Phase 3**: Gradually migrate controllers to use shared logic
4. **Phase 4**: Remove duplicated code after validation

## Alternative Approaches Considered

### 1. Code Generation
**Pros**: Could generate both specs from a template
**Cons**: Adds build complexity, harder to debug
**Decision**: Rejected in favor of struct embedding

### 2. Helper Functions Only
**Pros**: Simpler, no interface needed
**Cons**: Doesn't solve spec duplication, loose coupling
**Decision**: Rejected - doesn't address root cause

### 3. Single Unified Spec
**Pros**: Ultimate deduplication
**Cons**: Breaks CRD separation, confusing for users
**Decision**: Rejected - CRD separation is a feature

## Conclusion

This refactoring will significantly improve code maintainability while preserving backward compatibility and type safety. The shared reconciliation logic and embedded common spec provide a clean, extensible architecture for ArgoCD operator development.
