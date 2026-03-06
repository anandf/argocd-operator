package redis

import (
	"path/filepath"
	"runtime"
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

func newTestArgoCD() *argoproj.ArgoCD {
	return &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd",
			Namespace: "argocd",
		},
	}
}

func TestGolden_Standalone_ServiceAccount(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		})

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "serviceaccount.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "standalone", "serviceaccount.yaml"))
}

func TestGolden_Standalone_Deployment(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		}).
		WithServiceAccount("argocd-redis").
		WithImage("public.ecr.aws/docker/library/redis:7.0.15-alpine").
		WithExtra("ImagePullPolicy", "IfNotPresent").
		WithExtra("UseTLS", false).
		WithExtra("IsOpenShift", false)

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "deployment.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "standalone", "deployment.yaml"))
}

func TestGolden_Standalone_Service(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		})

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "service.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "standalone", "service.yaml"))
}

func TestGolden_Standalone_Deployment_TLS(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "redis").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		}).
		WithServiceAccount("argocd-redis").
		WithImage("public.ecr.aws/docker/library/redis:7.0.15-alpine").
		WithExtra("ImagePullPolicy", "IfNotPresent").
		WithExtra("UseTLS", true).
		WithExtra("TLSSecretName", "argocd-operator-redis-tls").
		WithExtra("IsOpenShift", false)

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "deployment.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "standalone", "deployment-tls.yaml"))
}
