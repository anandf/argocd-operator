package reposerver

import (
	"os"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	corev1 "k8s.io/api/core/v1"
)

// getRepoServerContainerImage returns the container image for the repo server.
// Priority: cr.Spec.Repo.Image > cr.Spec.Image > ARGOCD_IMAGE env > defaults
func getRepoServerContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false

	img := cr.Spec.Repo.Image
	if img == "" {
		img = cr.Spec.Image
	}
	if img == "" {
		img = common.ArgoCDDefaultArgoImage
		defaultImg = true
	}

	tag := cr.Spec.Repo.Version
	if tag == "" {
		tag = cr.Spec.Version
	}
	if tag == "" {
		tag = common.ArgoCDDefaultArgoVersion
		defaultTag = true
	}

	if e := os.Getenv(common.ArgoCDImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getRepoServerResources returns the ResourceRequirements for the repo server container.
func getRepoServerResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.Repo.Resources != nil {
		resources = *cr.Spec.Repo.Resources
	}
	return resources
}

// getRepoServerCommand builds the command line arguments for the repo server.
func getRepoServerCommand(cr *argoproj.ArgoCD) []string {
	cmd := []string{
		"uid_entrypoint.sh",
		"argocd-repo-server",
	}

	if cr.Spec.Redis.IsEnabled() {
		cmd = append(cmd, "--redis", cr.Name+"-redis."+cr.Namespace+".svc.cluster.local:6379")
	}

	cmd = append(cmd, "--loglevel", component.GetLogLevel(cr.Spec.Repo.LogLevel))
	cmd = append(cmd, "--logformat", component.GetLogFormat(cr.Spec.Repo.LogFormat))

	// Append extra args from CR spec
	if len(cr.Spec.Repo.ExtraRepoCommandArgs) > 0 {
		cmd = append(cmd, cr.Spec.Repo.ExtraRepoCommandArgs...)
	}

	return cmd
}

