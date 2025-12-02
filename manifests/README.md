# Template-Based Component Architecture

This directory contains YAML templates for Argo CD components. The templates are rendered using Go's `text/template` with [Sprig functions](http://masterminds.github.io/sprig/) for enhanced template capabilities.

## Directory Structure

```
manifests/
├── base/                          # Base templates (shared across platforms)
│   ├── server/                    # Server component templates
│   │   ├── deployment.yaml        # Server Deployment
│   │   ├── service.yaml           # Server Service
│   │   ├── service-metrics.yaml   # Server Metrics Service
│   │   ├── serviceaccount.yaml    # Server ServiceAccount
│   │   ├── role.yaml              # Server Role
│   │   └── rolebinding.yaml       # Server RoleBinding
│   ├── repo-server/               # Repository Server templates (TODO)
│   ├── application-controller/    # Application Controller templates (TODO)
│   ├── applicationset-controller/ # ApplicationSet Controller templates (TODO)
│   ├── redis/                     # Redis templates (TODO)
│   ├── dex/                       # Dex templates (TODO)
│   └── notifications-controller/  # Notifications Controller templates (TODO)
├── kubernetes/                    # Kubernetes-specific templates
│   └── server/
│       └── ingress.yaml          # Kubernetes Ingress
└── openshift/                     # OpenShift-specific templates
    └── server/
        └── route.yaml            # OpenShift Route
```

## How It Works

### 1. Template Rendering Engine

The template rendering engine is located in `pkg/component/template/engine.go`. It:
- Embeds all YAML templates using Go's `embed.FS`
- Renders templates using `text/template` with Sprig functions
- Converts rendered YAML to Kubernetes objects
- Provides a fluent API for building template data

### 2. Template Data Structure

Templates receive a `TemplateData` struct with the following fields:

```go
type TemplateData struct {
    CR          interface{}            // ArgoCD custom resource
    Namespace   string                 // Target namespace
    Name        string                 // ArgoCD instance name
    Component   string                 // Component name (e.g., "server")
    Labels      map[string]string      // Labels to apply
    Annotations map[string]string      // Annotations to apply
    ServiceAccount string              // ServiceAccount name
    Image       string                 // Container image
    Version     string                 // ArgoCD version
    Extra       map[string]interface{} // Component-specific extra data
}
```

### 3. Component Controllers

Each component has a `TemplateBasedController` that:
1. Builds template data from the ArgoCD CR
2. Renders base templates (ServiceAccount, Role, RoleBinding, Deployment, Service)
3. Renders platform-specific templates (Ingress/Route)
4. Applies decorators (SCC, resource limits, monitoring)
5. Creates/updates resources in the cluster

## Template Syntax

Templates use Go's `text/template` syntax with Sprig functions.

### Common Patterns

#### Accessing Template Data
```yaml
# Simple field access
name: {{ .Name }}-{{ .Component }}
namespace: {{ .Namespace }}

# Accessing nested fields
image: {{ .Image }}
replicas: {{ .Extra.Replicas | default 1 }}
```

#### Iterating Over Maps
```yaml
labels:
  {{- range $key, $value := .Labels }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
```

#### Conditionals
```yaml
{{- if .Annotations }}
annotations:
  {{- range $key, $value := .Annotations }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
{{- end }}
```

#### Default Values
```yaml
replicas: {{ .Extra.Replicas | default 1 }}
imagePullPolicy: {{ .Extra.ImagePullPolicy | default "Always" }}
serviceType: {{ .Extra.ServiceType | default "ClusterIP" }}
```

#### Complex Conditionals
```yaml
{{- if .Extra.IngressEnabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
# ... ingress spec
{{- end }}
```

### Sprig Functions

The templates support all [Sprig functions](http://masterminds.github.io/sprig/), including:

- **String functions**: `upper`, `lower`, `trim`, `quote`, `nindent`
- **Type conversion**: `toString`, `toInt`, `toBool`
- **Default values**: `default`, `empty`, `coalesce`
- **Encoding**: `b64enc`, `b64dec`
- **Date/time**: `now`, `date`
- **And many more...**

Example:
```yaml
# Base64 encode a value
secretData: {{ .Extra.SecretValue | b64enc }}

# Convert to upper case
env:
  - name: COMPONENT
    value: {{ .Component | upper }}
```

## Adding a New Component

To add a new component:

### 1. Create Template Directory

```bash
mkdir -p manifests/base/my-component
```

### 2. Create Base Templates

Create the following templates in `manifests/base/my-component/`:
- `serviceaccount.yaml`
- `role.yaml`
- `rolebinding.yaml`
- `deployment.yaml` or `statefulset.yaml`
- `service.yaml`
- Any other component-specific resources

### 3. Create Platform-Specific Templates (if needed)

If your component needs platform-specific resources:
```bash
mkdir -p manifests/kubernetes/my-component
mkdir -p manifests/openshift/my-component
```

### 4. Update Component Controller

Add a new method to `template_controller.go` to populate component-specific data:

```go
func (r *TemplateBasedController) addMyComponentData(cr *argoproj.ArgoCD, data *template.TemplateData) {
    // Set image
    data.WithImage(getMyComponentImage(cr))

    // Add extra data
    data.WithExtra("Replicas", getMyComponentReplicas(cr))

    // Add component-specific configuration
    if cr.Spec.MyComponent != nil {
        data.WithExtra("SomeConfig", cr.Spec.MyComponent.Config)
    }
}
```

### 5. Register in addComponentSpecificData

Update the switch statement in `addComponentSpecificData`:

```go
func (r *TemplateBasedController) addComponentSpecificData(...) {
    switch r.Component {
    // ... existing cases
    case "my-component":
        r.addMyComponentData(cr, data)
    }
}
```

## Template Examples

### ServiceAccount Template
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value | quote }}
    {{- end }}
```

### Deployment Template
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value | quote }}
    {{- end }}
spec:
  replicas: {{ .Extra.Replicas | default 1 }}
  selector:
    matchLabels:
      app.kubernetes.io/name: argocd-{{ .Component }}
      app.kubernetes.io/instance: {{ .Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: argocd-{{ .Component }}
        app.kubernetes.io/instance: {{ .Name }}
    spec:
      serviceAccountName: {{ .ServiceAccount }}
      containers:
        - name: {{ .Component }}
          image: {{ .Image }}
          {{- if .Extra.Resources }}
          resources:
            {{- if .Extra.Resources.Limits }}
            limits:
              {{- range $key, $value := .Extra.Resources.Limits }}
              {{ $key }}: {{ $value }}
              {{- end }}
            {{- end }}
            {{- if .Extra.Resources.Requests }}
            requests:
              {{- range $key, $value := .Extra.Resources.Requests }}
              {{ $key }}: {{ $value }}
              {{- end }}
            {{- end }}
          {{- end }}
```

### Service Template
```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value | quote }}
    {{- end }}
spec:
  type: {{ .Extra.ServiceType | default "ClusterIP" }}
  selector:
    app.kubernetes.io/name: argocd-{{ .Component }}
    app.kubernetes.io/instance: {{ .Name }}
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 8080
```

## Benefits of Template-Based Approach

### 1. **Separation of Concerns**
- Templates define **what** resources look like
- Controllers define **when** and **how** to create them
- Decorators define **platform-specific modifications**

### 2. **Easier Maintenance**
- YAML is easier to read and understand than Go code
- Changes to resource definitions don't require code changes
- Templates can be validated with standard YAML tools

### 3. **Platform Flexibility**
- Base templates work for all platforms
- Platform-specific templates only when needed
- Runtime API detection for optional features

### 4. **Better Testing**
- Templates can be tested independently
- Golden file testing for rendered manifests
- Easier to verify correctness

### 5. **Version Control**
- Clear diff when resource definitions change
- Easy to review changes in pull requests
- Template changes are self-documenting

## Migration from Legacy Code

The legacy code in `controllers/argocd/` programmatically creates resources. The migration process:

1. **Identify resource creation code** (e.g., `reconcileServerDeployment`)
2. **Extract resource definition** into a YAML template
3. **Extract configuration logic** into `addComponentData` methods
4. **Replace legacy reconcile function** with template-based controller
5. **Test thoroughly** to ensure behavior is preserved

### Example Migration

**Legacy code** (controllers/argocd/deployment.go):
```go
func (r *ReconcileArgoCD) reconcileServerDeployment(cr *argoproj.ArgoCD) error {
    deploy := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("%s-server", cr.Name),
            Namespace: cr.Namespace,
            Labels: map[string]string{
                "app": "argocd-server",
            },
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &replicas,
            // ... lots of code
        },
    }
    // ... create/update logic
}
```

**New approach**:

Template (`manifests/base/server/deployment.yaml`):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value | quote }}
    {{- end }}
spec:
  replicas: {{ .Extra.Replicas | default 1 }}
  # ... rest of spec
```

Controller:
```go
func (r *TemplateBasedController) addServerData(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) {
    data.WithImage(getArgoServerImage(cr))
    data.WithExtra("Replicas", getArgoServerReplicas(cr))
    // ... other configuration
}
```

## Future Enhancements

1. **Template Validation**: Pre-render templates during build and validate with `kubectl --dry-run`
2. **Template Testing**: Golden file tests for rendered templates
3. **Template Documentation**: Auto-generate documentation from templates
4. **Template Composition**: Support for template inheritance and composition
5. **Custom Functions**: Add operator-specific template functions

## Related Documentation

- [Go text/template](https://pkg.go.dev/text/template)
- [Sprig Functions](http://masterminds.github.io/sprig/)
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
- [ArgoCD Operator Architecture](../../docs/architecture/)
