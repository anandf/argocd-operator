# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Argo CD Operator is a Kubernetes operator for managing Argo CD clusters. Built using the Operator SDK (controller-runtime), it manages the full lifecycle of Argo CD instances via Custom Resource Definitions (CRDs).

**Main Branch:** `master`

### Operator Unification

This operator is being unified to support both vanilla Kubernetes and OpenShift platforms in a single codebase. Previously, there were separate operators:
- **argocd-operator** (Community): Kubernetes-focused
- **gitops-operator** (Red Hat): OpenShift-focused with additional features

The unified architecture uses:
- **Platform Abstraction**: Kubernetes and OpenShift platform implementations
- **Component-Based Controllers**: Each Argo CD component has its own controller
- **Decorator Pattern**: Platform-specific customizations applied via decorators

## Core Architecture

### API Versions
- **v1alpha1**: Legacy API version (being deprecated)
- **v1beta1**: Current API version with conversion webhook support
- CRDs: `ArgoCD`, `ArgoCDExport`, `NamespaceManagement`, `NotificationsConfiguration`

### Controllers
Located in `controllers/`:
- **argocd**: Main reconciler (`ReconcileArgoCD`) managing ArgoCD instances
  - Handles deployments for: application-controller, server, repo-server, dex, redis, notifications-controller, applicationset-controller
  - Component-specific reconciliation files: `deployment.go`, `dex.go`, `redis.go`, `applicationset.go`, `ingress.go`, `route.go`, etc.
  - Local user management with token renewal timers
- **argocdexport**: Manages export of ArgoCD resources
- **argocdagent**: Manages ArgoCD agent configurations
- **notificationsconfiguration**: Manages notifications
- **argoutil**: Utility functions including FIPS compliance checking

### Key Directories
- **api/**: CRD type definitions (v1alpha1, v1beta1)
- **common/**: Shared constants, defaults, keys, and values
- **config/**: Kubernetes manifests (CRDs, RBAC, manager, samples)
- **build/**: Dockerfiles and build utilities (redis configs, util image)
- **docs/**: Documentation (developer guides, usage, reference)
- **tests/**: E2E and integration tests
  - `tests/k8s/`: KUTTL-based E2E tests
  - `tests/ginkgo/`: Ginkgo-based tests (sequential and parallel)

### Platform-Specific Features
The operator detects and adapts to the platform:
- **OpenShift**: Routes, ClusterVersion API
- **Kubernetes**: Ingress resources
- **Optional**: Prometheus ServiceMonitor resources (if Prometheus Operator is installed)

Detection happens via `InspectCluster()` in the ArgoCD controller.

## Unified Architecture (New)

### Platform Abstraction Layer

Located in `pkg/platform/`, this layer provides platform-specific implementations:

**Platform Interface:**
```go
type Platform interface {
    PlatformParams() PlatformConfig
    AllSupportedControllers() ControllerMap
    AllSupportedDecorators() DecoratorMap
}
```

**Implementations:**
- `pkg/platform/platform_kubernetes.go`: Vanilla Kubernetes platform (default, no build tags)
- `pkg/platform/platform_openshift.go`: OpenShift platform with additional decorators (requires `-tags openshift`)
- `pkg/platform/api_detector.go`: Runtime API detection for resource types (Route vs Ingress, etc.)
- `pkg/platform/detector.go`: DEPRECATED - Old runtime platform detection (being phased out)

**Build-Time Platform Selection:**
The platform is determined at **build time** using Go build tags:
- **Kubernetes (default)**: `make build` or `go build`
- **OpenShift**: `make build-openshift` or `go build -tags=openshift`

This replaces the old runtime platform detection which was less reliable and created unnecessary coupling.

**Runtime API Detection:**
While the platform is selected at build time, the operator still performs **runtime API detection** for specific resource types:
- **Route vs Ingress**: Server component checks for Route API availability; if not found, uses Ingress
- **Gateway API**: Future support for Gateway API
- **ServiceMonitor**: Optional Prometheus monitoring if Prometheus Operator is installed

This approach provides:
1. **Build-time certainty**: Know exactly which decorators and platform-specific code is compiled in
2. **Runtime flexibility**: Automatically adapt to available APIs (e.g., create Route if available, otherwise Ingress)
3. **Better error handling**: Clear messages when required APIs are missing

### Component Controllers

Located in `pkg/component/`, each Argo CD component has a dedicated controller:

**Available Controllers:**
- `application_controller.go`: Argo CD Application Controller (StatefulSet)
- `applicationset.go`: ApplicationSet Controller
- `server.go`: Argo CD Server (API/UI)
- `reposerver.go`: Repository Server
- `redis.go`: Redis (standalone and HA modes)
- `dex.go`: Dex SSO (optional)
- `notifications.go`: Notifications Controller (optional)

**Controller Interface:**
```go
type Controller interface {
    Reconcile(cr *argoproj.ArgoCD, apiDetector *APIDetector) error
}
```

Each controller is responsible for:
1. Building template data from the ArgoCD CR
2. Rendering YAML templates for component resources
3. Applying decorators to rendered resources
4. Creating/updating resources in the cluster
5. Using APIDetector to check for available APIs (Route vs Ingress, ServiceMonitor, etc.)

**Template-Based Architecture (NEW):**

Component controllers now use YAML templates instead of programmatically creating resources:
- Templates located in `manifests/base/<component>/` for base resources
- Platform-specific templates in `manifests/kubernetes/` and `manifests/openshift/`
- Templates use Go's `text/template` with Sprig functions
- Template engine in `pkg/component/template/engine.go`
- Example implementation: `pkg/component/template_controller.go`

**Benefits:**
- Cleaner separation of resource definitions from reconciliation logic
- Easier to maintain and review resource changes
- Platform-specific resources clearly isolated
- Better testability with golden file testing

See `manifests/README.md` for detailed template documentation.

### Decorator Pattern

Located in `pkg/decorator/`, decorators modify Kubernetes objects before creation/update:

**Decorator Interface:**
```go
type Decorator interface {
    Decorate(obj runtime.Object) error
}
```

**Available Decorators:**
- `decorator.go` (SCCDecorator): OpenShift Security Context Constraints
- `resource_limits.go` (ResourceLimitsDecorator): Default resource limits/requests
- `monitoring.go` (MonitoringDecorator): Prometheus monitoring annotations
- `manager.go` (DecoratorManager): Orchestrates multiple decorators

**Decorator Application:**
- Applied before object creation/update
- Platform-specific (e.g., SCC only on OpenShift)
- Ordered execution (security → resources → monitoring)
- See `pkg/decorator/README.md` for detailed documentation

**Platform-Specific Decorators:**
- **Kubernetes**: Optional resource limits and monitoring
- **OpenShift**: SCC (required), optional resource limits and monitoring

### Migration from Legacy Architecture

The legacy architecture (`controllers/argocd/`) is being incrementally migrated to the new architecture:

**Legacy Pattern:**
```go
func (r *ReconcileArgoCD) reconcileServerDeployment(cr *argoproj.ArgoCD) error {
    // Monolithic reconciliation logic
}
```

**New Pattern:**
```go
// Platform instantiation (build-time selected)
platform := platform.NewPlatform(client, scheme)

// Create API detector for runtime API checks
apiDetector, err := platform.NewAPIDetector(client, config)

// Get component controller
serverController := platform.AllSupportedControllers()["server"]

// Reconcile component (controller uses apiDetector to check for Route/Ingress/etc)
err := serverController.Reconcile(cr, apiDetector)

// Apply decorators (platform-specific decorators compiled in at build time)
decoratorManager := platform.AllSupportedDecorators()
decoratorManager.Decorate(deployment)
```

**Migration Status:**

✅ **MIGRATION COMPLETE** - The template-based architecture is now fully implemented:

**Completed:**
1. **Template Engine** (`pkg/component/template/engine.go`)
   - Sprig v3 functions integrated
   - Fluent API for building template data
   - YAML to Kubernetes object conversion

2. **Component Templates** (all components migrated to templates):
   - `server`: Deployment, Service, ServiceAccount, Role, RoleBinding, Ingress/Route (`manifests/base/server/`)
   - `repo-server`: Deployment, Service, ServiceAccount (`manifests/base/repo-server/`)
   - `application-controller`: StatefulSet, Service, ServiceAccount (`manifests/base/application-controller/`)
   - `applicationset-controller`: Deployment, Service, ServiceAccount, Role, RoleBinding (`manifests/base/applicationset-controller/`)
   - `redis`: Deployment (standalone), StatefulSet (HA), Service, ServiceAccount (`manifests/base/redis/`)
   - `dex`: Deployment, Service, ServiceAccount (`manifests/base/dex/`)
   - `notifications-controller`: Deployment, Service, ServiceAccount, Role, RoleBinding (`manifests/base/notifications-controller/`)

3. **Platform-Specific Templates**:
   - Kubernetes Ingress (`manifests/kubernetes/server/ingress.yaml`)
   - OpenShift Route (`manifests/openshift/server/route.yaml`)

4. **Decorator Integration** (`pkg/component/decorator_manager.go`):
   - DecoratorManager for applying platform-specific modifications
   - Integrated with template controller reconciliation
   - Support for multiple decorators with ordered execution

5. **Tests** (`pkg/component/template/engine_test.go`, `pkg/component/decorator_manager_test.go`):
   - Template data builder tests
   - Decorator manager tests
   - Method chaining verification

6. **Documentation**:
   - Architecture guide (`docs/architecture/TEMPLATE_BASED_ARCHITECTURE.md`)
   - Template syntax reference (`manifests/README.md`)
   - Quick start guide (`manifests/QUICKSTART.md`)
   - Dependency documentation (`manifests/DEPENDENCIES.md`)

**Usage Example:**
```go
// Create controller with decorator support
decorators := NewDecoratorManager(
    NewSCCDecorator(client, scheme),        // OpenShift only
    NewResourceLimitsDecorator(),           // Optional
    NewMonitoringDecorator(),               // Optional
)

controller := NewTemplateBasedController(client, scheme, "server", platformType).
    WithDecorators(decorators)

// Reconcile component
err := controller.Reconcile(cr, apiDetector)
```

**Next Steps for Full Adoption:**
- Update main ArgoCD controller to use template-based controllers
- Migrate E2E tests to verify template-based resources
- Remove legacy resource creation code after validation
- Add golden file tests for template rendering output

### Architecture Diagrams

Visual representations of the architecture:
- `docs/architecture/architecture.drawio`: Current legacy architecture
- `docs/architecture/proposed_architecture.drawio`: Proposed unified architecture
- `docs/architecture/class_diagram.drawio`: Component class diagram showing Platform, Controller, and Decorator relationships

## Common Development Commands

### Building and Testing
```bash
# Build operator binary (default: Kubernetes platform)
make build

# Build for specific platforms
make build-kubernetes    # Kubernetes platform
make build-openshift     # OpenShift platform
make build-all-platforms # Both platforms

# Run unit tests (excludes E2E tests)
make test

# Run linter
make lint

# Run security scanner (gosec)
make gosec

# Format code
make fmt

# Generate CRDs and deepcopy code
make manifests generate
```

### Local Development
```bash
# Install CRDs and run operator locally (default: Kubernetes)
make install run

# Run with specific platform
make run-kubernetes  # Kubernetes platform
make run-openshift   # OpenShift platform

# Run with specific log level (debug, info, warn, error)
LOG_LEVEL=debug make run

# Run with custom namespace watching
WATCH_NAMESPACE=my-namespace make run

# Run with cluster config namespaces for E2E
ARGOCD_CLUSTER_CONFIG_NAMESPACES="argocd-e2e-cluster-config,argocd-test-impersonation-1-046,argocd-agent-principal-1-051" make run
```

### E2E Testing

**Prerequisites:** KUTTL CLI, kubectl/oc, jq, curl, GNU grep (macOS users: `brew install grep`)

```bash
# Run all E2E tests with KUTTL
make e2e

# Run operator locally and execute E2E tests
make all

# Run E2E tests manually
kubectl kuttl test ./tests/k8s --config ./tests/kuttl-tests.yaml

# Run single E2E test
kubectl kuttl test ./tests/k8s --config ./tests/kuttl-tests.yaml --test 1-004_validate_namespace_scoped_install

# Keep namespace after test failure for debugging
kubectl kuttl test ./tests/k8s --config ./tests/kuttl-tests.yaml --test <test-name> --skip-delete

# Run Ginkgo E2E tests (sequential)
make e2e-tests-sequential-ginkgo

# Run Ginkgo E2E tests (parallel, 5 procs)
make e2e-tests-parallel-ginkgo
```

**Note:** Redis HA tests require at least 3 worker nodes. Create local cluster: `k3d cluster create --servers 3`

### Container Images
```bash
# Build operator image (Docker or Podman auto-detected, default: Kubernetes)
make docker-build

# Build for specific platforms
make docker-build-kubernetes  # Kubernetes platform
make docker-build-openshift   # OpenShift platform

# Push operator image
make docker-push

# Override image name
make docker-build IMG=quay.io/myorg/argocd-operator:latest

# Build utility image (for backup)
make util-build

# Build bundle for OLM
make bundle bundle-build bundle-push
```

### Deployment
```bash
# Install CRDs
make install

# Uninstall CRDs
make uninstall

# Deploy operator to cluster
make deploy

# Undeploy operator
make undeploy
```

## Important Implementation Details

### Controller Reconciliation Pattern
The `ReconcileArgoCD` controller follows this pattern:
1. Fetch ArgoCD CR
2. Validate label selector (filters which ArgoCD instances to reconcile)
3. Reconcile each component in order:
   - RBAC (ServiceAccounts, Roles, ClusterRoles)
   - ConfigMaps (argocd-cm, argocd-rbac-cm, etc.)
   - Secrets
   - Deployments (application-controller, repo-server, server, dex, redis, notifications, applicationset)
   - Services
   - Ingress/Routes
   - Prometheus ServiceMonitors (if available)

Each component has a dedicated reconcile function (e.g., `reconcileApplicationController()`, `reconcileRedis()`).

### Label Selector
The operator can be scoped to reconcile only ArgoCD instances matching a label selector:
- Set via `--label-selector` flag or `ARGOCD_LABEL_SELECTOR` env var
- Default: matches all instances
- Format: `key=value` (standard Kubernetes label selector)

### FIPS Compliance
The operator detects FIPS-enabled environments and sets `GODEBUG=fips140=on` automatically via `FipsConfigChecker`.

### Local Users & Token Renewal
The operator manages local ArgoCD user tokens and automatically renews them using timers stored in `LocalUsersInfo.TokenRenewalTimers`. Thread-safe access via `UserTokensLock`.

### Conversion Webhook
v1alpha1 ↔ v1beta1 conversion webhook enabled via `ENABLE_CONVERSION_WEBHOOK=true` environment variable.

### Namespace Management
The operator can watch:
- All namespaces (default)
- Single namespace via `WATCH_NAMESPACE`
- Multiple namespaces via comma-separated `WATCH_NAMESPACE` list

### Ko Build Support
A `.ko.yaml` file is present for building with [ko](https://github.com/google/ko). Base image overrides are configured for UBI8-based builds.

## Testing Guidelines

### Unit Tests
- Run with `make test`
- Use `REDIS_CONFIG_PATH` pointing to `build/redis` for Redis-related tests
- Exclude E2E tests (pattern: `!/tests/ginkgo`)

### KUTTL E2E Tests
- Tests in `tests/k8s/` named `<ID>_<description>`
- Steps: `XX-<name>.yaml` (01, 02, etc.)
- Reserved filenames: `XX-assert.yaml`, `XX-errors.yaml`
- Use `TestStep` with scripts for complex assertions
- Always document tests with inline comments and `README.md`

### Ginkgo Tests
- Located in `tests/ginkgo/sequential/` and `tests/ginkgo/parallel/`
- Use dot imports for Ginkgo/Gomega (whitelisted in `.golangci.yml`)
- Run with `make e2e-tests-sequential-ginkgo` or `make e2e-tests-parallel-ginkgo`

## Updating Default Argo CD Version

1. **Update CRDs**: Copy from [upstream Argo CD manifests/crds](https://github.com/argoproj/argo-cd/tree/master/manifests/crds) to `config/crd/bases`
2. **Update Image Hash**: Modify `ArgoCDDefaultArgoVersion` in `common/defaults.go`
3. Test thoroughly with new version

## Code Style

- **Linting**: `golangci-lint` v2.3.0 (run with `make lint`)
- **Imports**: Use `goimports` with local prefix `github.com/argoproj-labs/argocd-operator`
- **Ginkgo/Gomega**: Dot imports allowed (see `.golangci.yml`)
- **Generated Code**: Controller-gen v0.18.0 for deepcopy and CRD generation

## Makefile Variables

Common overrides:
- `VERSION`: Operator version (default: 0.17.0)
- `IMG`: Operator image URL (default: `quay.io/argoprojlabs/argocd-operator:v$(VERSION)`)
- `BUNDLE_IMG`: Bundle image URL
- `OPERATOR_SDK_VERSION`: v1.35.0
- `CONTAINER_RUNTIME`: Auto-detected (Docker or Podman)

## Troubleshooting

### Controller Name Validation
With controller-runtime v0.19.0+, unique controller names are enforced. The operator uses `SkipNameValidation: true` to bypass this (we have non-unique names historically).

### Conversion Webhook for Local Testing
When running `make install` locally, the conversion webhook is stripped from CRDs (see Makefile line 160) to avoid webhook call failures during local development.

### FIPS Cluster Detection
If builds fail in FIPS environments, check `FipsConfigChecker` implementation in `controllers/argoutil`.

## Documentation

- **Online Docs**: https://argocd-operator.readthedocs.io
- **Local Docs**: `mkdocs serve` (requires `pip3 install mkdocs mkdocs-material`)
- **Contributing**: See `docs/developer-guide/contributing.md`
