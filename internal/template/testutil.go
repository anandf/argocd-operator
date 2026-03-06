package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

// AssertGoldenFile renders a template with the given data and compares the result
// against a golden file. If the UPDATE_GOLDEN environment variable is set to "true",
// the golden file is created/updated instead of compared.
func AssertGoldenFile(t *testing.T, engine *TemplateEngine, templatePath string, data interface{}, goldenPath string) {
	t.Helper()

	obj, err := engine.RenderManifest(templatePath, data)
	if err != nil {
		t.Fatalf("failed to render template %s: %v", templatePath, err)
	}

	rendered, err := MarshalObject(obj)
	if err != nil {
		t.Fatalf("failed to marshal rendered object: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN") == "true" {
		dir := filepath.Dir(goldenPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create golden file directory: %v", err)
		}
		if err := os.WriteFile(goldenPath, rendered, 0o644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s (run with UPDATE_GOLDEN=true to create): %v", goldenPath, err)
	}

	if string(rendered) != string(expected) {
		t.Errorf("rendered template does not match golden file %s\n\nGot:\n%s\n\nExpected:\n%s",
			goldenPath, string(rendered), string(expected))
	}
}

// MarshalObject converts a client.Object to YAML bytes for golden file comparison.
func MarshalObject(obj interface{}) ([]byte, error) {
	// First marshal to JSON (handles the unstructured object), then convert to YAML
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, err
	}

	return yamlBytes, nil
}
