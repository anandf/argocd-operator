# Decorator Package

The decorator package provides a flexible way to modify Kubernetes objects before they are created or updated. Decorators implement the Decorator interface and can be applied to various Kubernetes resource types.

## Decorator Interface

```go
type Decorator interface {
    Decorate(obj runtime.Object) error
}
```

## Available Decorators

### 1. SCCDecorator (OpenShift)
Applies Security Context Constraints (SCC) configurations to pod specs for OpenShift compliance.

**Use Case:** OpenShift platform-specific security requirements
**Applies To:** Pods, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs
**Configuration:** Automatically sets RuntimeDefault seccomp profile

### 2. ResourceLimitsDecorator
Applies default resource limits and requests to containers if not already specified.

**Use Case:** Ensuring resource constraints for all workloads
**Applies To:** Pods, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs
**Configuration:**
- Default CPU Limit: 500m
- Default Memory Limit: 256Mi
- Default CPU Request: 250m
- Default Memory Request: 128Mi

### 3. MonitoringDecorator
Adds Prometheus monitoring annotations and labels to enable metrics scraping.

**Use Case:** Enabling Prometheus metrics collection
**Applies To:** Pods, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs
**Configuration:**
- Annotations: `prometheus.io/scrape`, `prometheus.io/port`, `prometheus.io/path`
- Labels: `app.kubernetes.io/component`, `monitoring`

## Decorator Manager

The `DecoratorManager` orchestrates the application of multiple decorators to objects.

### Usage Example

```go
// Create decorators
sccDecorator := decorator.NewSCCDecorator(client, scheme)
resourceDecorator := decorator.NewResourceLimitsDecorator(client, scheme, argoCD, "server")
monitoringDecorator := decorator.NewMonitoringDecorator(client, scheme, argoCD, "server")

// Create manager with decorators
manager := decorator.NewDecoratorManager(sccDecorator, resourceDecorator, monitoringDecorator)

// Apply decorators to an object
deployment := &appsv1.Deployment{...}
err := manager.Decorate(deployment)
```

## Decorator Application Strategy

### When Decorators Are Applied

Decorators are applied at the following points in the reconciliation lifecycle:

1. **Pre-Create**: Before creating a new Kubernetes object
2. **Pre-Update**: Before updating an existing Kubernetes object

### Decorator Ordering

Decorators are applied in the order they are registered with the DecoratorManager:

1. **Security Decorators** (e.g., SCCDecorator) - Applied first for compliance
2. **Resource Decorators** (e.g., ResourceLimitsDecorator) - Applied second for resource management
3. **Monitoring Decorators** (e.g., MonitoringDecorator) - Applied last for observability

### Platform-Specific Decorators

Decorators can be platform-specific:

**Kubernetes Platform:**
- ResourceLimitsDecorator (optional)
- MonitoringDecorator (optional)

**OpenShift Platform:**
- SCCDecorator (required)
- ResourceLimitsDecorator (optional)
- MonitoringDecorator (optional)

### Component-Specific Decorators

Decorators can also be component-specific. For example:
- Server components may require specific resource limits
- ApplicationSet may need different monitoring configurations

## Creating Custom Decorators

To create a custom decorator:

1. Implement the `Decorator` interface
2. Handle the runtime.Object type assertions for supported resource types
3. Modify the object in-place using pointers
4. Return an error if decoration fails

Example:

```go
type CustomDecorator struct {
    Client client.Client
    Scheme *runtime.Scheme
    logger logr.Logger
}

func (d *CustomDecorator) Decorate(obj runtime.Object) error {
    var podspec *corev1.PodSpec

    switch typed := obj.(type) {
    case *appsv1.Deployment:
        podspec = &typed.Spec.Template.Spec
    case *appsv1.StatefulSet:
        podspec = &typed.Spec.Template.Spec
    default:
        return fmt.Errorf("unsupported type: %T", obj)
    }

    // Modify podspec
    podspec.DNSPolicy = corev1.DNSClusterFirst

    return nil
}
```

## Best Practices

1. **Idempotency**: Decorators should be idempotent - applying the same decorator multiple times should have the same effect as applying it once
2. **Conditional Application**: Check if configuration already exists before applying defaults
3. **Error Handling**: Return errors when decoration fails, don't silently skip
4. **Logging**: Use structured logging to track decorator application
5. **Type Safety**: Always use type assertions with the comma-ok idiom
6. **Pointer Usage**: Always use pointers when modifying nested structures to ensure changes persist

## Future Enhancements

Potential future decorators:

- **NetworkPolicyDecorator**: Applies network policies for component isolation
- **AffinityDecorator**: Adds pod affinity/anti-affinity rules
- **TolerationsDecorator**: Adds tolerations for node taints
- **OAuthDecorator**: Configures OAuth integration for OpenShift
- **RouteDecorator**: Configures OpenShift Routes for external access
