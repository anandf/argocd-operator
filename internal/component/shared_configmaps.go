package component

import (
	"context"
	"embed"
	"fmt"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

//go:embed shared_templates/*.tmpl
var sharedTemplateFS embed.FS

var sharedLog = logs.Log.WithName("shared-configmaps")

var sharedTemplateEngine = template.NewTemplateEngine(sharedTemplateFS, "shared_templates")

// ReconcileSharedConfigMaps ensures that shared ArgoCD ConfigMaps exist.
// These are ConfigMaps referenced by multiple component deployments as volumes:
// - argocd-ssh-known-hosts-cm (SSH known hosts for git repos)
// - argocd-tls-certs-cm (custom TLS certificates for git repos)
// - argocd-gpg-keys-cm (GPG keys for commit verification)
//
// This must be called BEFORE component reconciliation to ensure volumes can mount.
func ReconcileSharedConfigMaps(ctx context.Context, c client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD) error {
	log := sharedLog.WithValues("argocd", cr.Name, "namespace", cr.Namespace)
	log.Info("reconciling shared configmaps")

	if err := reconcileSSHKnownHostsConfigMap(ctx, c, scheme, cr); err != nil {
		return fmt.Errorf("failed to reconcile ssh-known-hosts configmap: %w", err)
	}

	if err := reconcileTLSCertsConfigMap(ctx, c, scheme, cr); err != nil {
		return fmt.Errorf("failed to reconcile tls-certs configmap: %w", err)
	}

	if err := reconcileGPGKeysConfigMap(ctx, c, scheme, cr); err != nil {
		return fmt.Errorf("failed to reconcile gpg-keys configmap: %w", err)
	}

	return nil
}

// reconcileSSHKnownHostsConfigMap ensures the argocd-ssh-known-hosts-cm ConfigMap exists.
// Uses create-only semantics: if the ConfigMap already exists, it will not be overwritten
// to preserve any user edits to the SSH known hosts data.
func reconcileSSHKnownHostsConfigMap(ctx context.Context, c client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD) error {
	existing := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: common.ArgoCDKnownHostsConfigMapName, Namespace: cr.Namespace}
	err := c.Get(ctx, key, existing)
	if err == nil {
		// Already exists — ensure owner reference is set
		return ensureOwnerReference(ctx, c, scheme, cr, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Build SSH known hosts data from CR spec
	sshKnownHosts := getInitialSSHKnownHosts(cr)

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "").
		WithLabels(argoutil.LabelsForCluster(cr)).
		WithExtra("SSHKnownHosts", sshKnownHosts)

	obj, err := sharedTemplateEngine.RenderManifest("ssh-known-hosts-cm.yaml.tmpl", data)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := template.ConvertToTyped(obj, cm); err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(cr, cm, scheme); err != nil {
		return err
	}

	sharedLog.Info("creating shared configmap", "name", cm.Name, "namespace", cm.Namespace)
	return c.Create(ctx, cm)
}

// reconcileTLSCertsConfigMap ensures the argocd-tls-certs-cm ConfigMap exists.
// Uses create-only semantics to preserve user-added TLS certificates.
func reconcileTLSCertsConfigMap(ctx context.Context, c client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD) error {
	existing := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: common.ArgoCDTLSCertsConfigMapName, Namespace: cr.Namespace}
	err := c.Get(ctx, key, existing)
	if err == nil {
		return ensureOwnerReference(ctx, c, scheme, cr, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "").
		WithLabels(argoutil.LabelsForCluster(cr))

	// Add initial TLS certs from CR spec if provided
	if len(cr.Spec.TLS.InitialCerts) > 0 {
		data.WithExtra("TLSCerts", cr.Spec.TLS.InitialCerts)
	}

	obj, err := sharedTemplateEngine.RenderManifest("tls-certs-cm.yaml.tmpl", data)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := template.ConvertToTyped(obj, cm); err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(cr, cm, scheme); err != nil {
		return err
	}

	sharedLog.Info("creating shared configmap", "name", cm.Name, "namespace", cm.Namespace)
	return c.Create(ctx, cm)
}

// reconcileGPGKeysConfigMap ensures the argocd-gpg-keys-cm ConfigMap exists.
// Created as an empty ConfigMap; users populate it with GPG keys as needed.
func reconcileGPGKeysConfigMap(ctx context.Context, c client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD) error {
	existing := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: common.ArgoCDGPGKeysConfigMapName, Namespace: cr.Namespace}
	err := c.Get(ctx, key, existing)
	if err == nil {
		return ensureOwnerReference(ctx, c, scheme, cr, existing)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	data := template.NewTemplateData(cr, cr.Namespace, cr.Name, "").
		WithLabels(argoutil.LabelsForCluster(cr))

	obj, err := sharedTemplateEngine.RenderManifest("gpg-keys-cm.yaml.tmpl", data)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := template.ConvertToTyped(obj, cm); err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(cr, cm, scheme); err != nil {
		return err
	}

	sharedLog.Info("creating shared configmap", "name", cm.Name, "namespace", cm.Namespace)
	return c.Create(ctx, cm)
}

// ensureOwnerReference sets the controller reference on an existing ConfigMap if missing.
func ensureOwnerReference(ctx context.Context, c client.Client, scheme *runtime.Scheme, cr *argoproj.ArgoCD, cm *corev1.ConfigMap) error {
	existingRefs := cm.GetOwnerReferences()
	for _, ref := range existingRefs {
		if ref.UID == cr.UID {
			return nil // Already has correct owner
		}
	}

	if err := controllerutil.SetControllerReference(cr, cm, scheme); err != nil {
		return err
	}

	sharedLog.Info("updating owner reference on shared configmap", "name", cm.Name, "namespace", cm.Namespace)
	return c.Update(ctx, cm)
}

// getInitialSSHKnownHosts returns the SSH known hosts data based on CR spec.
func getInitialSSHKnownHosts(cr *argoproj.ArgoCD) string {
	skh := common.ArgoCDDefaultSSHKnownHosts
	if cr.Spec.InitialSSHKnownHosts.ExcludeDefaultHosts {
		skh = ""
	}
	if len(cr.Spec.InitialSSHKnownHosts.Keys) > 0 {
		skh += cr.Spec.InitialSSHKnownHosts.Keys
	}
	return skh
}
