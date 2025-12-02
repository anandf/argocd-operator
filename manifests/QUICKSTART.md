# Template-Based Controllers - Quick Start Guide

This guide shows you how to use the template-based architecture for creating Argo CD component controllers.

## 5-Minute Tutorial

### Step 1: Create Your Templates

Create a directory for your component under `manifests/base/`:

```bash
mkdir -p manifests/base/my-component
```

Create a ServiceAccount template:
```yaml
# manifests/base/my-component/serviceaccount.yaml
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

Create a Deployment template:
```yaml
# manifests/base/my-component/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}-{{ .Component }}
  namespace: {{ .Namespace }}
spec:
  replicas: {{ .Extra.Replicas | default 1 }}
  selector:
    matchLabels:
      app: {{ .Name }}-{{ .Component }}
  template:
    metadata:
      labels:
        app: {{ .Name }}-{{ .Component }}
    spec:
      serviceAccountName: {{ .ServiceAccount }}
      containers:
        - name: {{ .Component }}
          image: {{ .Image }}
```

### Step 2: Create Component-Specific Data Builder

Add a method to build data for your component:

```go
// pkg/component/template_controller.go

func (r *TemplateBasedController) addMyComponentData(cr *argoproj.ArgoCD, data *template.TemplateData) {
    // Set image
    image := "myregistry/my-component:latest"
    if cr.Spec.MyComponent != nil && cr.Spec.MyComponent.Image != "" {
        image = cr.Spec.MyComponent.Image
    }
    data.WithImage(image)

    // Set replicas
    replicas := int32(1)
    if cr.Spec.MyComponent != nil && cr.Spec.MyComponent.Replicas != nil {
        replicas = *cr.Spec.MyComponent.Replicas
    }
    data.WithExtra("Replicas", replicas)

    // Add any other component-specific configuration
    if cr.Spec.MyComponent != nil {
        if len(cr.Spec.MyComponent.Env) > 0 {
            data.WithExtra("Env", cr.Spec.MyComponent.Env)
        }
    }
}
```

Register your component in the switch statement:

```go
func (r *TemplateBasedController) addComponentSpecificData(...) {
    switch r.Component {
    // ... existing cases
    case "my-component":
        r.addMyComponentData(cr, data)
    }
}
```

### Step 3: Use the Controller

Create and use your controller:

```go
// In your reconciliation code
controller := NewTemplateBasedController(
    r.Client,
    r.Scheme,
    "my-component",  // component name
    platformType,     // "kubernetes" or "openshift"
)

err := controller.Reconcile(cr, apiDetector)
if err != nil {
    return err
}
```

That's it! Your component will now be managed using templates.

## Common Patterns

### Pattern 1: Conditional Resources

Use template conditionals to optionally render resources:

```yaml
{{- if .Extra.Enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}-{{ .Component }}
# ... service spec
{{- end }}
```

In your data builder:
```go
data.WithExtra("Enabled", cr.Spec.MyComponent.Enabled)
```

### Pattern 2: Dynamic Port Lists

```yaml
ports:
  {{- range .Extra.Ports }}
  - name: {{ .Name }}
    port: {{ .Port }}
    targetPort: {{ .TargetPort }}
  {{- end }}
```

In your data builder:
```go
ports := []map[string]interface{}{
    {"Name": "http", "Port": 8080, "TargetPort": 8080},
    {"Name": "metrics", "Port": 8081, "TargetPort": 8081},
}
data.WithExtra("Ports", ports)
```

### Pattern 3: Environment Variables from Spec

```yaml
env:
  {{- range .Extra.Env }}
  - name: {{ .Name }}
    {{- if .Value }}
    value: {{ .Value | quote }}
    {{- else if .ValueFrom }}
    valueFrom:
      {{- if .ValueFrom.SecretKeyRef }}
      secretKeyRef:
        name: {{ .ValueFrom.SecretKeyRef.Name }}
        key: {{ .ValueFrom.SecretKeyRef.Key }}
      {{- end }}
    {{- end }}
  {{- end }}
```

In your data builder:
```go
data.WithExtra("Env", cr.Spec.MyComponent.Env)
```

### Pattern 4: Resource Limits/Requests

```yaml
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

In your data builder:
```go
if cr.Spec.MyComponent.Resources != nil {
    data.WithExtra("Resources", map[string]interface{}{
        "Limits":   cr.Spec.MyComponent.Resources.Limits,
        "Requests": cr.Spec.MyComponent.Resources.Requests,
    })
}
```

### Pattern 5: Platform-Specific Resources

Create platform-specific template:
```yaml
# manifests/openshift/my-component/route.yaml
{{- if .Extra.RouteEnabled }}
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: {{ .Name }}-{{ .Component }}
spec:
  to:
    kind: Service
    name: {{ .Name }}-{{ .Component }}
{{- end }}
```

In controller:
```go
func (r *TemplateBasedController) reconcilePlatformResources(...) {
    if r.Component == "my-component" && apiDetector.HasRoute(ctx) {
        templatePath := "openshift/my-component/route.yaml"
        obj, _ := r.engine.RenderManifest(templatePath, data)
        r.reconcileResource(ctx, cr, obj)
    }
}
```

## Testing Your Templates

### Unit Test for Data Building

```go
func TestMyComponentData(t *testing.T) {
    cr := &argoproj.ArgoCD{
        Spec: argoproj.ArgoCDSpec{
            MyComponent: &argoproj.MyComponentSpec{
                Image:    "custom-image:v1.0.0",
                Replicas: int32Ptr(3),
            },
        },
    }

    controller := NewTemplateBasedController(nil, nil, "my-component", "kubernetes")
    data := controller.buildTemplateData(cr, nil)

    assert.Equal(t, "custom-image:v1.0.0", data.Image)
    assert.Equal(t, int32(3), data.Extra["Replicas"])
}
```

### Golden File Test for Rendering

```go
func TestMyComponentTemplateRendering(t *testing.T) {
    data := &template.TemplateData{
        Name:           "test-argocd",
        Namespace:      "test-ns",
        Component:      "my-component",
        ServiceAccount: "test-sa",
        Image:          "test-image:latest",
        Labels: map[string]string{
            "app": "test",
        },
        Extra: map[string]interface{}{
            "Replicas": 2,
        },
    }

    engine := template.NewTemplateEngine(manifestsFS, "manifests")
    obj, err := engine.RenderManifest("base/my-component/deployment.yaml", data)
    require.NoError(t, err)

    // Compare with golden file
    expected := readGoldenFile(t, "testdata/my-component-deployment.yaml")
    actual := marshalToYAML(obj)
    assert.YAMLEq(t, expected, actual)
}
```

## Debugging Templates

### View Rendered Output

Use this helper to see what your template renders to:

```go
import "sigs.k8s.io/yaml"

// In your test or debug code
obj, _ := engine.RenderManifest("base/my-component/deployment.yaml", data)
yamlBytes, _ := yaml.Marshal(obj)
fmt.Println(string(yamlBytes))
```

### Common Template Errors

**Error: "template: ... undefined"**
- Accessing a field that doesn't exist in TemplateData
- Solution: Check field name spelling, ensure data is set with `WithExtra`

**Error: "template: ... invalid type for range"**
- Trying to range over non-map/slice
- Solution: Check that Extra field is correct type

**Error: "template: ... nil pointer evaluating"**
- Accessing nested field without nil check
- Solution: Add conditional: `{{- if .Extra.Field }}`

**Error: "yaml: unmarshal errors"**
- Invalid YAML syntax in rendered output
- Solution: Check template indentation, use `nindent` for nested YAML

## Best Practices

1. **Keep Templates Simple**: Complex logic should be in Go code, not templates
2. **Use Defaults**: Always provide sensible defaults with `| default`
3. **Nil Checks**: Check for nil before accessing nested fields
4. **Quote Strings**: Use `| quote` for string values in labels/annotations
5. **Consistent Naming**: Use `{{ .Name }}-{{ .Component }}` pattern
6. **Document Extra Fields**: Comment what Extra fields your component uses
7. **Test Both Paths**: Test with and without optional fields

## Next Steps

- Read the full [Template Architecture Documentation](../docs/architecture/TEMPLATE_BASED_ARCHITECTURE.md)
- Explore [Sprig Functions](http://masterminds.github.io/sprig/)
- See existing templates in `manifests/base/server/`
- Review examples in `pkg/component/template/example_test.go`

## Getting Help

- Check `manifests/README.md` for detailed template syntax
- Look at existing components for reference
- File issues at https://github.com/argoproj-labs/argocd-operator/issues
