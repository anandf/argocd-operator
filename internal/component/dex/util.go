package dex

import (
	"os"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	corev1 "k8s.io/api/core/v1"
)

// getDexContainerImage returns the container image for the Dex server.
func getDexContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false

	img := ""
	tag := ""

	if cr.Spec.SSO != nil && cr.Spec.SSO.Dex != nil && cr.Spec.SSO.Dex.Image != "" {
		img = cr.Spec.SSO.Dex.Image
	}

	if img == "" {
		img = common.ArgoCDDefaultDexImage
		defaultImg = true
	}

	if cr.Spec.SSO != nil && cr.Spec.SSO.Dex != nil && cr.Spec.SSO.Dex.Version != "" {
		tag = cr.Spec.SSO.Dex.Version
	}

	if tag == "" {
		tag = common.ArgoCDDefaultDexVersion
		defaultTag = true
	}

	if e := os.Getenv(common.ArgoCDDexImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getDexResources returns the ResourceRequirements for the Dex container.
func getDexResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.SSO != nil && cr.Spec.SSO.Dex != nil && cr.Spec.SSO.Dex.Resources != nil {
		resources = *cr.Spec.SSO.Dex.Resources
	}
	return resources
}
