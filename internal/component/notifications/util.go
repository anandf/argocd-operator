package notifications

import (
	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	corev1 "k8s.io/api/core/v1"
)

// getNotificationsContainerImage returns the container image for the Notifications controller.
// Notifications uses the main ArgoCD image since it is part of the argocd binary.
func getNotificationsContainerImage(cr *argoproj.ArgoCD) string {
	// Check CR-level notification image overrides first
	if cr.Spec.Notifications.Image != "" || cr.Spec.Notifications.Version != "" {
		img := cr.Spec.Notifications.Image
		if img == "" {
			img = cr.Spec.Image
			if img == "" {
				return component.GetArgoContainerImage(cr)
			}
		}
		tag := cr.Spec.Notifications.Version
		if tag == "" {
			tag = cr.Spec.Version
			if tag == "" {
				return component.GetArgoContainerImage(cr)
			}
		}
		return img + ":" + tag
	}
	return component.GetArgoContainerImage(cr)
}

// getNotificationsResources returns the ResourceRequirements for the Notifications controller.
func getNotificationsResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.Notifications.Resources != nil {
		resources = *cr.Spec.Notifications.Resources
	}
	return resources
}

// getNotificationsCommand builds the command arguments for the Notifications controller.
func getNotificationsCommand(cr *argoproj.ArgoCD) []string {
	cmd := []string{
		"argocd-notifications",
	}

	cmd = append(cmd, "--loglevel", component.GetLogLevel(cr.Spec.Notifications.LogLevel))
	cmd = append(cmd, "--logformat", component.GetLogFormat(cr.Spec.Notifications.LogFormat))

	return cmd
}
