package redis

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"text/template"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed scripts/*.tpl
var scriptsFS embed.FS

// scriptParams holds the parameters used when rendering configuration scripts.
type scriptParams struct {
	ServiceName string
	UseTLS      string
}

// getRedisContainerImage returns the container image for the Redis server.
func getRedisContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false
	img := cr.Spec.Redis.Image
	if img == "" {
		img = common.ArgoCDDefaultRedisImage
		defaultImg = true
	}
	tag := cr.Spec.Redis.Version
	if tag == "" {
		tag = common.ArgoCDDefaultRedisVersion
		defaultTag = true
	}
	if e := os.Getenv(common.ArgoCDRedisImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getRedisHAContainerImage returns the container image for Redis in HA mode.
func getRedisHAContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false
	img := cr.Spec.Redis.Image
	if img == "" {
		img = common.ArgoCDDefaultRedisImage
		defaultImg = true
	}
	tag := cr.Spec.Redis.Version
	if tag == "" {
		tag = common.ArgoCDDefaultRedisVersionHA
		defaultTag = true
	}
	if e := os.Getenv(common.ArgoCDRedisHAImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getRedisHAProxyContainerImage returns the container image for HAProxy.
func getRedisHAProxyContainerImage(cr *argoproj.ArgoCD) string {
	defaultImg, defaultTag := false, false
	img := cr.Spec.HA.RedisProxyImage
	if img == "" {
		img = common.ArgoCDDefaultRedisHAProxyImage
		defaultImg = true
	}
	tag := cr.Spec.HA.RedisProxyVersion
	if tag == "" {
		tag = common.ArgoCDDefaultRedisHAProxyVersion
		defaultTag = true
	}
	if e := os.Getenv(common.ArgoCDRedisHAProxyImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getRedisResources returns the ResourceRequirements for the Redis container.
func getRedisResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.Redis.Resources != nil {
		resources = *cr.Spec.Redis.Resources
	}
	return resources
}

// getRedisHAResources returns the ResourceRequirements for Redis HA components.
func getRedisHAResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if cr.Spec.HA.Resources != nil {
		resources = *cr.Spec.HA.Resources
	}
	return resources
}

// getRedisHAReplicas returns the number of replicas for Redis HA.
func getRedisHAReplicas() int32 {
	return common.ArgoCDDefaultRedisHAReplicas
}

// redisShouldUseTLS checks if TLS should be enabled for Redis by looking for the TLS secret.
func redisShouldUseTLS(c client.Client, cr *argoproj.ArgoCD) bool {
	var tlsSecretObj corev1.Secret
	tlsSecretName := types.NamespacedName{Namespace: cr.Namespace, Name: common.ArgoCDRedisServerTLSSecretName}
	err := c.Get(context.TODO(), tlsSecretName, &tlsSecretObj)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "error looking up redis tls secret")
		}
		return false
	}

	secretOwnerRefs := tlsSecretObj.GetOwnerReferences()
	if len(secretOwnerRefs) > 0 {
		// OpenShift service CA makes the owner reference for the TLS secret to the
		// service, which in turn is owned by the controller. This method performs
		// a lookup of the controller through the intermediate owning service.
		for _, secretOwner := range secretOwnerRefs {
			if secretOwner.Kind == "Service" {
				key := client.ObjectKey{Name: secretOwner.Name, Namespace: tlsSecretObj.GetNamespace()}
				svc := &corev1.Service{}

				err := c.Get(context.TODO(), key, svc)
				if err != nil {
					log.Error(err, "could not get owner of secret", "secret", tlsSecretObj.GetName())
					return false
				}

				serviceOwnerRefs := svc.GetOwnerReferences()
				for _, serviceOwner := range serviceOwnerRefs {
					if serviceOwner.Kind == "ArgoCD" {
						return true
					}
				}
			}
		}
	} else {
		// For secrets without owner (i.e. manually created), we apply some
		// heuristics based on the annotation.
		if _, ok := tlsSecretObj.Annotations[common.AnnotationName]; ok {
			return true
		}
	}
	return false
}

// renderConfigScript renders a configuration script template from the embedded scripts FS.
func renderConfigScript(scriptName string, params scriptParams) (string, error) {
	content, err := scriptsFS.ReadFile(fmt.Sprintf("scripts/%s", scriptName))
	if err != nil {
		return "", fmt.Errorf("failed to read script %s: %w", scriptName, err)
	}

	tmpl, err := template.New(scriptName).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse script template %s: %w", scriptName, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute script template %s: %w", scriptName, err)
	}

	return buf.String(), nil
}

// tlsString returns "true" or "false" as a string for use in script templates.
func tlsString(useTLS bool) string {
	if useTLS {
		return "true"
	}
	return "false"
}
