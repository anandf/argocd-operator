package server

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

func TestGolden_Server_ServiceAccount(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "server").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		})

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "serviceaccount.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "serviceaccount.yaml"))
}

func TestGolden_Server_Deployment(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "server").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		}).
		WithServiceAccount("argocd-server").
		WithImage("quay.io/argoproj/argocd:v2.7.0").
		WithExtra("ImagePullPolicy", "IfNotPresent").
		WithExtra("Command", []string{"--insecure", "--staticassets", "/shared/app", "--loglevel", "info", "--logformat", "text"})

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "deployment.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "deployment.yaml"))
}

func TestGolden_Server_Service(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "server").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		}).
		WithExtra("ServiceType", "ClusterIP")

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "service.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "service.yaml"))
}

func TestGolden_Server_MetricsService(t *testing.T) {
	cr := newTestArgoCD()
	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "server").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithAnnotations(map[string]string{
			"argocds.argoproj.io/name":      cr.Name,
			"argocds.argoproj.io/namespace": cr.Namespace,
		})

	engine := template.NewTemplateEngine(templateFS, "templates")
	template.AssertGoldenFile(t, engine, "service-metrics.yaml.tmpl", data,
		filepath.Join(testDir(), "testdata", "service-metrics.yaml"))
}
