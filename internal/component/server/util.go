package server

import (
	"os"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	corev1 "k8s.io/api/core/v1"
)

// getServerContainerImage returns the container image for the ArgoCD server.
// Priority: cr.Spec.Image > ARGOCD_IMAGE env > defaults
func getServerContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false

	img := cr.Spec.Image
	if img == "" {
		img = common.ArgoCDDefaultArgoImage
		defaultImg = true
	}

	tag := cr.Spec.Version
	if tag == "" {
		tag = common.ArgoCDDefaultArgoVersion
		defaultTag = true
	}

	if e := os.Getenv(common.ArgoCDImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getServerResources returns the ResourceRequirements for the server container.
func getServerResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.Server.Resources != nil {
		resources = *cr.Spec.Server.Resources
	}
	return resources
}

// getServerCommand builds the command line arguments for the ArgoCD server.
func getServerCommand(cr *argoproj.ArgoCD) []string {
	cmd := []string{}

	if cr.Spec.Server.Insecure {
		cmd = append(cmd, "--insecure")
	}

	cmd = append(cmd, "--staticassets", "/shared/app")

	cmd = append(cmd, "--dex-server", cr.Name+"-dex-server."+cr.Namespace+".svc.cluster.local:5556")

	if cr.Spec.Repo.IsEnabled() {
		cmd = append(cmd, "--repo-server", cr.Name+"-repo-server."+cr.Namespace+".svc.cluster.local:8081")
	}

	if cr.Spec.Redis.IsEnabled() {
		cmd = append(cmd, "--redis", cr.Name+"-redis."+cr.Namespace+".svc.cluster.local:6379")
	}

	cmd = append(cmd, "--loglevel", component.GetLogLevel(cr.Spec.Server.LogLevel))
	cmd = append(cmd, "--logformat", component.GetLogFormat(cr.Spec.Server.LogFormat))

	// Append extra command args from CR spec
	if len(cr.Spec.Server.ExtraCommandArgs) > 0 {
		cmd = append(cmd, cr.Spec.Server.ExtraCommandArgs...)
	}

	return cmd
}

// getServerServiceType returns the Kubernetes service type for the server.
func getServerServiceType(cr *argoproj.ArgoCD) string {
	if cr.Spec.Server.Service.Type != "" {
		return string(cr.Spec.Server.Service.Type)
	}
	return "ClusterIP"
}

// getServerHost returns the host for the server Ingress/Route.
func getServerHost(cr *argoproj.ArgoCD) string {
	if cr.Spec.Server.Host != "" {
		return cr.Spec.Server.Host
	}
	return cr.Name
}

// getPathOrDefault returns the path or the default "/" if empty.
func getPathOrDefault(path string) string {
	if path != "" {
		return path
	}
	return common.ArgoCDDefaultIngressPath
}
