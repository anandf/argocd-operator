/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterargocd

import (
	"context"
	"fmt"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileSourceNamespaces reconciles source namespaces for ClusterArgoCD
// This includes:
// - Labeling namespaces with managed-by-cluster-argocd label
// - Creating Roles/RoleBindings in source namespaces for ArgoCD server access
// - Reconciling ApplicationSet and Notifications source namespaces
func (r *ReconcileClusterArgoCD) reconcileSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling source namespaces for ClusterArgoCD", "name", clusterArgoCD.Name)

	// Initialize maps if needed
	if r.ManagedSourceNamespaces == nil {
		r.ManagedSourceNamespaces = make(map[string]string)
	}
	if r.ManagedApplicationSetSourceNamespaces == nil {
		r.ManagedApplicationSetSourceNamespaces = make(map[string]string)
	}
	if r.ManagedNotificationsSourceNamespaces == nil {
		r.ManagedNotificationsSourceNamespaces = make(map[string]string)
	}

	// Reconcile Application source namespaces
	if err := r.reconcileApplicationSourceNamespaces(clusterArgoCD); err != nil {
		return fmt.Errorf("failed to reconcile application source namespaces: %w", err)
	}

	// Reconcile ApplicationSet source namespaces
	if err := r.reconcileApplicationSetSourceNamespaces(clusterArgoCD); err != nil {
		return fmt.Errorf("failed to reconcile applicationset source namespaces: %w", err)
	}

	// Reconcile Notifications source namespaces
	if err := r.reconcileNotificationsSourceNamespaces(clusterArgoCD); err != nil {
		return fmt.Errorf("failed to reconcile notifications source namespaces: %w", err)
	}

	// Clean up namespaces that are no longer managed
	if err := r.cleanupUnmanagedSourceNamespaces(clusterArgoCD); err != nil {
		return fmt.Errorf("failed to cleanup unmanaged source namespaces: %w", err)
	}

	return nil
}

// reconcileApplicationSourceNamespaces handles source namespaces for Applications
func (r *ReconcileClusterArgoCD) reconcileApplicationSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	if len(clusterArgoCD.Spec.SourceNamespaces) == 0 {
		return nil
	}

	managedByLabel := getClusterArgoCDManagedByLabel(clusterArgoCD)

	for _, namespace := range clusterArgoCD.Spec.SourceNamespaces {
		// Get the namespace
		ns := &corev1.Namespace{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Info("source namespace not found", "namespace", namespace, "clusterArgoCD", clusterArgoCD.Name)
				continue
			}
			return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
		}

		// Add managed-by label to the namespace
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}

		labelKey := getClusterArgoCDManagedByLabelKey()
		if ns.Labels[labelKey] != managedByLabel {
			ns.Labels[labelKey] = managedByLabel
			log.Info("adding managed-by label to namespace", "namespace", namespace, "label", managedByLabel)
			if err := r.Update(context.TODO(), ns); err != nil {
				return fmt.Errorf("failed to update namespace %s labels: %w", namespace, err)
			}
		}

		// Track this namespace
		r.ManagedSourceNamespaces[namespace] = clusterArgoCD.Name

		// Create Role and RoleBinding in the source namespace for ArgoCD server
		if err := r.createRoleInSourceNamespace(namespace, clusterArgoCD); err != nil {
			return fmt.Errorf("failed to create role in namespace %s: %w", namespace, err)
		}
	}

	return nil
}

// reconcileApplicationSetSourceNamespaces handles source namespaces for ApplicationSets
func (r *ReconcileClusterArgoCD) reconcileApplicationSetSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	if clusterArgoCD.Spec.ApplicationSet == nil || len(clusterArgoCD.Spec.ApplicationSet.SourceNamespaces) == 0 {
		return nil
	}

	managedByLabel := getClusterArgoCDManagedByLabel(clusterArgoCD)
	labelKey := getApplicationSetManagedByLabelKey()

	for _, namespace := range clusterArgoCD.Spec.ApplicationSet.SourceNamespaces {
		ns := &corev1.Namespace{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Info("applicationset source namespace not found", "namespace", namespace)
				continue
			}
			return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
		}

		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}

		if ns.Labels[labelKey] != managedByLabel {
			ns.Labels[labelKey] = managedByLabel
			log.Info("adding applicationset managed-by label to namespace", "namespace", namespace)
			if err := r.Update(context.TODO(), ns); err != nil {
				return fmt.Errorf("failed to update namespace %s labels: %w", namespace, err)
			}
		}

		r.ManagedApplicationSetSourceNamespaces[namespace] = clusterArgoCD.Name
	}

	return nil
}

// reconcileNotificationsSourceNamespaces handles source namespaces for Notifications
func (r *ReconcileClusterArgoCD) reconcileNotificationsSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	if !clusterArgoCD.Spec.Notifications.Enabled || len(clusterArgoCD.Spec.Notifications.SourceNamespaces) == 0 {
		return nil
	}

	managedByLabel := getClusterArgoCDManagedByLabel(clusterArgoCD)
	labelKey := getNotificationsManagedByLabelKey()

	for _, namespace := range clusterArgoCD.Spec.Notifications.SourceNamespaces {
		ns := &corev1.Namespace{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Info("notifications source namespace not found", "namespace", namespace)
				continue
			}
			return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
		}

		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}

		if ns.Labels[labelKey] != managedByLabel {
			ns.Labels[labelKey] = managedByLabel
			log.Info("adding notifications managed-by label to namespace", "namespace", namespace)
			if err := r.Update(context.TODO(), ns); err != nil {
				return fmt.Errorf("failed to update namespace %s labels: %w", namespace, err)
			}
		}

		r.ManagedNotificationsSourceNamespaces[namespace] = clusterArgoCD.Name
	}

	return nil
}

// createRoleInSourceNamespace creates a Role in the source namespace for ArgoCD server
func (r *ReconcileClusterArgoCD) createRoleInSourceNamespace(namespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	roleName := generateResourceName(common.ArgoCDServerComponent, clusterArgoCD, namespace)

	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels:    getLabelsForClusterArgoCD(clusterArgoCD),
		},
		Rules: getServerSourceNamespacePolicyRules(),
	}

	existingRole := &v1.Role{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: roleName, Namespace: namespace}, existingRole)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// Role doesn't exist, create it
		log.Info("creating role in source namespace", "namespace", namespace, "role", roleName)
		return r.Create(context.TODO(), role)
	}

	// Role exists, update if needed
	existingRole.Rules = role.Rules
	return r.Update(context.TODO(), existingRole)
}

// cleanupUnmanagedSourceNamespaces removes labels from namespaces that are no longer managed
func (r *ReconcileClusterArgoCD) cleanupUnmanagedSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	// Get all namespaces with the managed-by label
	managedByLabel := getClusterArgoCDManagedByLabel(clusterArgoCD)
	labelKey := getClusterArgoCDManagedByLabelKey()

	namespaceList := &corev1.NamespaceList{}
	listOption := client.MatchingLabels{
		labelKey: managedByLabel,
	}

	if err := r.List(context.TODO(), namespaceList, listOption); err != nil {
		return fmt.Errorf("failed to list managed namespaces: %w", err)
	}

	// Build a map of currently configured source namespaces
	currentSourceNamespaces := make(map[string]bool)
	for _, ns := range clusterArgoCD.Spec.SourceNamespaces {
		currentSourceNamespaces[ns] = true
	}
	if clusterArgoCD.Spec.ApplicationSet != nil {
		for _, ns := range clusterArgoCD.Spec.ApplicationSet.SourceNamespaces {
			currentSourceNamespaces[ns] = true
		}
	}
	if clusterArgoCD.Spec.Notifications.Enabled {
		for _, ns := range clusterArgoCD.Spec.Notifications.SourceNamespaces {
			currentSourceNamespaces[ns] = true
		}
	}

	// Remove labels from namespaces that are no longer in the spec
	for _, ns := range namespaceList.Items {
		if !currentSourceNamespaces[ns.Name] {
			delete(ns.Labels, labelKey)
			log.Info("removing managed-by label from namespace", "namespace", ns.Name)
			if err := r.Update(context.TODO(), &ns); err != nil {
				log.Error(err, "failed to remove label from namespace", "namespace", ns.Name)
			}
		}
	}

	return nil
}

// getServerSourceNamespacePolicyRules returns policy rules for ArgoCD server in source namespaces
func getServerSourceNamespacePolicyRules() []v1.PolicyRule {
	return []v1.PolicyRule{
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"applications"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"applicationsets"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{"appprojects"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		},
		// TODO: Add more policy rules as needed based on argocd controller
	}
}
