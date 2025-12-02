package template

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TemplateEngine renders Kubernetes manifests from templates
type TemplateEngine struct {
	// templatesFS is the embedded filesystem containing the templates
	templatesFS embed.FS
	// basePath is the base path within the embedded filesystem
	basePath string
}

// NewTemplateEngine creates a new template engine with the given embedded filesystem
func NewTemplateEngine(templatesFS embed.FS, basePath string) *TemplateEngine {
	return &TemplateEngine{
		templatesFS: templatesFS,
		basePath:    basePath,
	}
}

// RenderManifest renders a single template file with the given data
func (e *TemplateEngine) RenderManifest(templatePath string, data interface{}) (client.Object, error) {
	// Read the template file
	content, err := e.templatesFS.ReadFile(filepath.Join(e.basePath, templatePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}

	// Create a new template with sprig functions
	tmpl, err := template.New(filepath.Base(templatePath)).
		Funcs(sprig.TxtFuncMap()).
		Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	// Execute the template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}

	// Decode the rendered YAML into an Unstructured object
	obj := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(&buf, 4096)
	if err := decoder.Decode(obj); err != nil {
		return nil, fmt.Errorf("failed to decode rendered template %s: %w", templatePath, err)
	}

	return obj, nil
}

// RenderManifests renders multiple template files from a directory
func (e *TemplateEngine) RenderManifests(templateDir string, data interface{}) ([]client.Object, error) {
	var objects []client.Object

	// Walk the template directory
	dirPath := filepath.Join(e.basePath, templateDir)
	err := fs.WalkDir(e.templatesFS, dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-YAML files
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}

		// Get relative path from basePath
		relPath, err := filepath.Rel(e.basePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Render the manifest
		obj, err := e.RenderManifest(relPath, data)
		if err != nil {
			return err
		}

		objects = append(objects, obj)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to render manifests from %s: %w", templateDir, err)
	}

	return objects, nil
}

// ConvertToTyped converts an unstructured object to a typed object
func ConvertToTyped(obj client.Object, into runtime.Object) error {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("object is not unstructured")
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, into)
}

// TemplateData is the data structure passed to templates
type TemplateData struct {
	// CR is the ArgoCD custom resource
	CR interface{}
	// Namespace is the namespace where resources will be created
	Namespace string
	// Name is the name of the ArgoCD instance
	Name string
	// Component is the component name (e.g., "server", "repo-server")
	Component string
	// Labels are the labels to apply to resources
	Labels map[string]string
	// Annotations are the annotations to apply to resources
	Annotations map[string]string
	// ServiceAccount is the service account name
	ServiceAccount string
	// Image is the container image to use
	Image string
	// Version is the ArgoCD version
	Version string
	// Extra contains component-specific extra data
	Extra map[string]interface{}
}

// NewTemplateData creates a new TemplateData with common fields populated
func NewTemplateData(cr interface{}, namespace, name, component string) *TemplateData {
	return &TemplateData{
		CR:          cr,
		Namespace:   namespace,
		Name:        name,
		Component:   component,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Extra:       make(map[string]interface{}),
	}
}

// WithLabels adds labels to the template data
func (d *TemplateData) WithLabels(labels map[string]string) *TemplateData {
	for k, v := range labels {
		d.Labels[k] = v
	}
	return d
}

// WithAnnotations adds annotations to the template data
func (d *TemplateData) WithAnnotations(annotations map[string]string) *TemplateData {
	for k, v := range annotations {
		d.Annotations[k] = v
	}
	return d
}

// WithServiceAccount sets the service account name
func (d *TemplateData) WithServiceAccount(sa string) *TemplateData {
	d.ServiceAccount = sa
	return d
}

// WithImage sets the container image
func (d *TemplateData) WithImage(image string) *TemplateData {
	d.Image = image
	return d
}

// WithVersion sets the ArgoCD version
func (d *TemplateData) WithVersion(version string) *TemplateData {
	d.Version = version
	return d
}

// WithExtra adds extra data for component-specific customization
func (d *TemplateData) WithExtra(key string, value interface{}) *TemplateData {
	d.Extra[key] = value
	return d
}
