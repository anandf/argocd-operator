# Template-Based Component Architecture

## Overview

The ArgoCD Operator is transitioning from programmatically creating Kubernetes resources in Go code to using YAML templates with a template rendering engine. This document describes the architecture, rationale, and implementation details.

## Table of Contents

1. [Motivation](#motivation)
2. [Architecture](#architecture)
3. [Components](#components)
4. [Template System](#template-system)
5. [Migration Guide](#migration-guide)
6. [Testing Strategy](#testing-strategy)
7. [Examples](#examples)

## Motivation

### Problems with Programmatic Resource Creation

The legacy approach programmatically creates Kubernetes resources in Go:

```go
func (r *ReconcileArgoCD) reconcileServerDeployment(cr *argoproj.ArgoCD) error {
    deploy := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("%s-server", cr.Name),
            Namespace: cr.Namespace,
            Labels: map[string]string{
                "app": "argocd-server",
                "component": "server",
            },
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: getServerReplicas(cr),
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "app": "argocd-server",
                },
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{
                        "app": "argocd-server",
                    },
                },
                Spec: corev1.PodSpec{
                    ServiceAccountName: "argocd-server",
                    Containers: []corev1.Container{
                        {
                            Name: "argocd-server",
                            Image: getServerImage(cr),
                            // ... hundreds of lines of code
                        },
                    },
                },
            },
        },
    }
    // ... create/update logic
}
```

**Issues:**
1. **Hard to Read**: Go structs are verbose and difficult to visualize
2. **Hard to Maintain**: Changes require modifying Go code and recompiling
3. **Hard to Review**: Diffs in pull requests are difficult to understand
4. **No Separation**: Resource definitions mixed with reconciliation logic
5. **Platform Coupling**: Platform-specific code scattered throughout

### Benefits of Template-Based Approach

```yaml
# manifests/base/server/deployment.yaml
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

**Advantages:**
1. **Readable**: Standard YAML that anyone can understand
2. **Maintainable**: Changes don't require code modifications
3. **Reviewable**: Clear diffs in pull requests
4. **Separated**: Templates separate from reconciliation logic
5. **Platform-Aware**: Platform-specific templates clearly isolated

## Architecture

### High-Level Flow

```
┌─────────────────┐
│  ArgoCD CR      │
│  (User Input)   │
└────────┬────────┘
         │
         v
┌─────────────────────────────┐
│  TemplateBasedController    │
│  1. Build template data     │
│  2. Render templates        │
│  3. Apply decorators        │
│  4. Create/update resources │
└────────┬────────────────────┘
         │
         v
┌─────────────────────────────┐
│  Template Engine            │
│  - Load templates from FS   │
│  - Render with data         │
│  - Convert to K8s objects   │
└────────┬────────────────────┘
         │
         v
┌─────────────────────────────┐
│  Decorator Manager          │
│  - Apply SCC (OpenShift)    │
│  - Apply resource limits    │
│  - Apply monitoring         │
└────────┬────────────────────┘
         │
         v
┌─────────────────────────────┐
│  Kubernetes API             │
│  - Create/update resources  │
└─────────────────────────────┘
```

### Directory Structure

```
argocd-operator/
├── manifests/                    # Embedded templates
│   ├── base/                     # Platform-agnostic templates
│   │   ├── server/
│   │   ├── repo-server/
│   │   ├── application-controller/
│   │   ├── applicationset-controller/
│   │   ├── redis/
│   │   ├── dex/
│   │   └── notifications-controller/
│   ├── kubernetes/               # Kubernetes-specific templates
│   │   └── server/
│   │       └── ingress.yaml
│   └── openshift/                # OpenShift-specific templates
│       └── server/
│           └── route.yaml
├── pkg/
│   ├── component/
│   │   ├── template/
│   │   │   └── engine.go         # Template rendering engine
│   │   ├── template_controller.go # Template-based controller
│   │   └── ... (legacy controllers)
│   ├── platform/                 # Platform abstraction
│   │   ├── detector.go
│   │   ├── api_detector.go
│   │   └── types.go
│   └── decorator/                # Resource decorators
│       ├── decorator.go
│       ├── resource_limits.go
│       └── monitoring.go
└── controllers/
    └── argocd/                   # Legacy controllers (being migrated)
```

## Components

### 1. Template Engine (`pkg/component/template/engine.go`)

**Responsibilities:**
- Load templates from embedded filesystem
- Render templates with provided data
- Convert rendered YAML to Kubernetes objects

**Key Features:**
- Uses Go's `text/template` with Sprig functions
- Supports single template or directory rendering
- Type conversion from Unstructured to typed objects

**Interface:**
```go
type TemplateEngine struct {
    templatesFS embed.FS
    basePath    string
}

func NewTemplateEngine(templatesFS embed.FS, basePath string) *TemplateEngine
func (e *TemplateEngine) RenderManifest(templatePath string, data interface{}) (client.Object, error)
func (e *TemplateEngine) RenderManifests(templateDir string, data interface{}) ([]client.Object, error)
```

### 2. Template Data Structure

**Purpose:** Provides structured data to templates

```go
type TemplateData struct {
    CR          interface{}            // ArgoCD custom resource
    Namespace   string                 // Target namespace
    Name        string                 // ArgoCD instance name
    Component   string                 // Component name
    Labels      map[string]string      // Labels to apply
    Annotations map[string]string      // Annotations to apply
    ServiceAccount string              // ServiceAccount name
    Image       string                 // Container image
    Version     string                 // ArgoCD version
    Extra       map[string]interface{} // Component-specific data
}
```

**Builder Pattern:**
```go
data := NewTemplateData(cr, namespace, name, component).
    WithLabels(labels).
    WithServiceAccount(saName).
    WithImage(image).
    WithExtra("Replicas", 3)
```

### 3. Template-Based Controller (`pkg/component/template_controller.go`)

**Responsibilities:**
- Build template data from ArgoCD CR
- Render base and platform-specific templates
- Apply decorators
- Create/update resources

**Key Methods:**
```go
func (r *TemplateBasedController) Reconcile(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) error
func (r *TemplateBasedController) buildTemplateData(cr *argoproj.ArgoCD, apiDetector *platform.APIDetector) *template.TemplateData
func (r *TemplateBasedController) reconcileBaseResources(cr *argoproj.ArgoCD, data *template.TemplateData) error
func (r *TemplateBasedController) reconcilePlatformResources(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) error
```

### 4. Decorator Integration

Decorators are applied after template rendering but before resource creation:

```go
// Render template
obj, err := r.engine.RenderManifest(templatePath, data)

// Apply decorators (platform-specific)
decoratorManager.Decorate(obj)

// Create/update resource
r.reconcileResource(ctx, cr, obj)
```

## Template System

### Template Syntax

Templates use Go's `text/template` with Sprig functions.

#### Basic Fields
```yaml
name: {{ .Name }}-{{ .Component }}
namespace: {{ .Namespace }}
image: {{ .Image }}
```

#### Map Iteration
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
serviceType: {{ .Extra.ServiceType | default "ClusterIP" }}
```

#### Complex Structures
```yaml
{{- if .Extra.Resources }}
resources:
  {{- if .Extra.Resources.Limits }}
  limits:
    {{- range $key, $value := .Extra.Resources.Limits }}
    {{ $key }}: {{ $value }}
    {{- end }}
  {{- end }}
{{- end }}
```

### Sprig Functions

All [Sprig functions](http://masterminds.github.io/sprig/) are available:

- **String**: `upper`, `lower`, `trim`, `quote`, `nindent`
- **Encoding**: `b64enc`, `b64dec`
- **Type Conversion**: `toString`, `toInt`
- **Default Values**: `default`, `coalesce`
- **Lists**: `list`, `append`, `concat`
- **Dicts**: `dict`, `set`, `unset`

Example:
```yaml
env:
  - name: COMPONENT_NAME
    value: {{ .Component | upper }}
  - name: CONFIG
    value: {{ .Extra.Config | b64enc }}
```

### Platform-Specific Templates

Use API detection to conditionally render platform-specific resources:

```go
// In controller
if cr.Spec.Server.Route.Enabled && apiDetector.HasRoute(ctx) {
    templatePath := "openshift/server/route.yaml"
    obj, _ := r.engine.RenderManifest(templatePath, data)
    r.reconcileResource(ctx, cr, obj)
}

if cr.Spec.Server.Ingress.Enabled && apiDetector.HasIngress(ctx) {
    templatePath := "kubernetes/server/ingress.yaml"
    obj, _ := r.engine.RenderManifest(templatePath, data)
    r.reconcileResource(ctx, cr, obj)
}
```

Template with conditional rendering:
```yaml
{{- if .Extra.RouteEnabled }}
apiVersion: route.openshift.io/v1
kind: Route
# ... route spec
{{- end }}
```

## Migration Guide

### Step 1: Identify Resource Creation Code

Find the legacy reconcile function (e.g., `reconcileServerDeployment`).

### Step 2: Extract Resource Definition

Create a YAML template from the Go struct:

**From:**
```go
deploy := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name: fmt.Sprintf("%s-server", cr.Name),
        Namespace: cr.Namespace,
    },
    Spec: appsv1.DeploymentSpec{
        Replicas: &replicas,
    },
}
```

**To:**
```yaml
# manifests/base/server/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
spec:
  replicas: {{ .Extra.Replicas | default 1 }}
```

### Step 3: Extract Configuration Logic

Move configuration logic to `addComponentData` method:

```go
func (r *TemplateBasedController) addServerData(cr *argoproj.ArgoCD, data *template.TemplateData, apiDetector *platform.APIDetector) {
    // Image
    data.WithImage(getArgoServerImage(cr))

    // Replicas
    data.WithExtra("Replicas", getArgoServerReplicas(cr))

    // Resources
    if cr.Spec.Server.Resources != nil {
        data.WithExtra("Resources", map[string]interface{}{
            "Limits":   cr.Spec.Server.Resources.Limits,
            "Requests": cr.Spec.Server.Resources.Requests,
        })
    }
}
```

### Step 4: Replace Legacy Controller

Replace direct resource creation with template rendering:

```go
// Old
deploy := newServerDeployment(cr)
r.Client.Create(ctx, deploy)

// New
data := r.buildTemplateData(cr, apiDetector)
r.reconcileBaseResources(cr, data)
```

### Step 5: Test

Ensure behavior is preserved:
1. Unit tests for template data building
2. Golden file tests for rendered templates
3. E2E tests to verify resources are created correctly

## Testing Strategy

### 1. Template Validation Tests

Test that templates are syntactically correct:

```go
func TestTemplateValidation(t *testing.T) {
    engine := template.NewTemplateEngine(manifestsFS, "manifests")
    data := template.NewTemplateData(cr, "test-ns", "test", "server")

    _, err := engine.RenderManifest("base/server/deployment.yaml", data)
    assert.NoError(t, err)
}
```

### 2. Golden File Tests

Test rendered output matches expected YAML:

```go
func TestServerDeploymentRendering(t *testing.T) {
    data := buildTestData()
    obj, _ := engine.RenderManifest("base/server/deployment.yaml", data)

    actual := marshalYAML(obj)
    expected := readGoldenFile("testdata/server-deployment.yaml")

    assert.Equal(t, expected, actual)
}
```

### 3. E2E Tests

Verify resources are created correctly in a real cluster (existing KUTTL tests).

## Examples

### Complete Server Component

#### ServiceAccount
```yaml
# manifests/base/server/serviceaccount.yaml
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

#### Deployment
```yaml
# manifests/base/server/deployment.yaml
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
        - name: argocd-{{ .Component }}
          image: {{ .Image }}
          {{- if .Extra.Resources }}
          resources:
            {{- toYaml .Extra.Resources | nindent 12 }}
          {{- end }}
```

#### Service
```yaml
# manifests/base/server/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
spec:
  type: {{ .Extra.ServiceType | default "ClusterIP" }}
  selector:
    app.kubernetes.io/name: argocd-{{ .Component }}
    app.kubernetes.io/instance: {{ .Name }}
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

#### Ingress (Kubernetes)
```yaml
# manifests/kubernetes/server/ingress.yaml
{{- if .Extra.IngressEnabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
spec:
  rules:
    - host: {{ .Extra.IngressHost }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ .Name }}-{{ .Component }}
                port:
                  name: http
{{- end }}
```

#### Route (OpenShift)
```yaml
# manifests/openshift/server/route.yaml
{{- if .Extra.RouteEnabled }}
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
spec:
  to:
    kind: Service
    name: {{ .Name }}-{{ .Component }}
  port:
    targetPort: https
  tls:
    termination: passthrough
{{- end }}
```

### Controller Implementation

```go
// Create controller
controller := NewTemplateBasedController(
    client,
    scheme,
    "server",        // component name
    "openshift",     // platform type
)

// Reconcile
err := controller.Reconcile(cr, apiDetector)
```

## Future Enhancements

1. **Template Composition**: Support for template inheritance/composition
2. **Validation**: Pre-render and validate templates during build
3. **Documentation**: Auto-generate docs from templates
4. **Custom Functions**: Add operator-specific template functions
5. **Hot Reload**: Support reloading templates without restart (dev mode)

## References

- [Go text/template](https://pkg.go.dev/text/template)
- [Sprig Functions](http://masterminds.github.io/sprig/)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Operator SDK Best Practices](https://sdk.operatorframework.io/docs/best-practices/)
