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

	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newClusterRole returns a new ClusterRole instance for ClusterArgoCD
func newClusterRole(name string, rules []v1.PolicyRule, clusterArgoCD *argoproj.ClusterArgoCD) *v1.ClusterRole {
	return &v1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:        generateClusterRoleName(name, clusterArgoCD),
			Labels:      getLabelsForClusterArgoCD(clusterArgoCD),
			Annotations: getAnnotationsForClusterArgoCD(clusterArgoCD),
		},
		Rules: rules,
	}
}

// reconcileClusterRoles ensures that all ClusterRoles for ClusterArgoCD are configured
func (r *ReconcileClusterArgoCD) reconcileClusterRoles(clusterArgoCD *argoproj.ClusterArgoCD) error {
	// Get the policy rules for different components
	// For now, we'll create placeholder ClusterRoles
	// Full implementation will mirror the logic from argocd controller

	log.Info("reconciling ClusterRoles for ClusterArgoCD", "name", clusterArgoCD.Name)

	// Reconcile application-controller ClusterRole
	if err := r.reconcileClusterRole(common.ArgoCDApplicationControllerComponent, getApplicationControllerPolicyRules(), clusterArgoCD); err != nil {
		return err
	}

	// Reconcile server ClusterRole
	if err := r.reconcileClusterRole(common.ArgoCDServerComponent, getServerPolicyRules(), clusterArgoCD); err != nil {
		return err
	}

	// TODO: Add more component ClusterRoles as needed (dex, redis, etc.)

	return nil
}

// reconcileClusterRole reconciles a single ClusterRole for a ClusterArgoCD component
func (r *ReconcileClusterArgoCD) reconcileClusterRole(componentName string, policyRules []v1.PolicyRule, clusterArgoCD *argoproj.ClusterArgoCD) error {
	clusterRole := newClusterRole(componentName, policyRules, clusterArgoCD)

	existingClusterRole := &v1.ClusterRole{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: clusterRole.Name}, existingClusterRole)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ClusterRole %s: %w", clusterRole.Name, err)
		}

		// ClusterRole doesn't exist, create it
		log.Info("creating ClusterRole", "name", clusterRole.Name)
		if err := r.Create(context.TODO(), clusterRole); err != nil {
			return fmt.Errorf("failed to create ClusterRole %s: %w", clusterRole.Name, err)
		}
		return nil
	}

	// ClusterRole exists, update if needed
	if !clusterRoleNeedsUpdate(existingClusterRole, clusterRole) {
		return nil
	}

	log.Info("updating ClusterRole", "name", clusterRole.Name)
	existingClusterRole.Rules = clusterRole.Rules
	existingClusterRole.Labels = clusterRole.Labels
	existingClusterRole.Annotations = clusterRole.Annotations

	if err := r.Update(context.TODO(), existingClusterRole); err != nil {
		return fmt.Errorf("failed to update ClusterRole %s: %w", clusterRole.Name, err)
	}

	return nil
}

// clusterRoleNeedsUpdate checks if a ClusterRole needs to be updated
func clusterRoleNeedsUpdate(existing, desired *v1.ClusterRole) bool {
	// Simple comparison - in production, this should be more sophisticated
	if len(existing.Rules) != len(desired.Rules) {
		return true
	}
	// TODO: Add more sophisticated comparison logic
	return false
}

// getLabelsForClusterArgoCD returns labels for ClusterArgoCD resources
func getLabelsForClusterArgoCD(clusterArgoCD *argoproj.ClusterArgoCD) map[string]string {
	labels := map[string]string{
		common.ArgoCDKeyName:      clusterArgoCD.Name,
		common.ArgoCDKeyPartOf:    common.ArgoCDAppName,
		common.ArgoCDKeyManagedBy: common.ArgoCDAppName,
	}
	// TODO: Merge with any user-provided labels from clusterArgoCD.Spec
	return labels
}

// getAnnotationsForClusterArgoCD returns annotations for ClusterArgoCD resources
func getAnnotationsForClusterArgoCD(clusterArgoCD *argoproj.ClusterArgoCD) map[string]string {
	annotations := make(map[string]string)
	// TODO: Add relevant annotations
	return annotations
}

// Placeholder policy rules - these should be expanded based on the actual requirements
// Full implementation will reference the policy rules from the argocd controller

func getApplicationControllerPolicyRules() []v1.PolicyRule {
	return []v1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"events"},
			Verbs:     []string{"create", "patch"},
		},
		// TODO: Add complete policy rules from argocd controller
	}
}

func getServerPolicyRules() []v1.PolicyRule {
	return []v1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
			Verbs:     []string{"get", "list", "watch"},
		},
		// TODO: Add complete policy rules from argocd controller
	}
}

// reconcileClusterRoleBinding reconciles a single ClusterRoleBinding for a component
func (r *ReconcileClusterArgoCD) reconcileClusterRoleBinding(componentName, targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	clusterRoleBindingName := generateClusterRoleName(componentName, clusterArgoCD)
	clusterRoleName := generateClusterRoleName(componentName, clusterArgoCD)
	serviceAccountName := getServiceAccountName(componentName, clusterArgoCD)

	desiredClusterRoleBinding := &v1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        clusterRoleBindingName,
			Labels:      getLabelsForClusterArgoCD(clusterArgoCD),
			Annotations: getAnnotationsForClusterArgoCD(clusterArgoCD),
		},
		RoleRef: v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
		Subjects: []v1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: targetNamespace,
			},
		},
	}

	existingClusterRoleBinding := &v1.ClusterRoleBinding{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: clusterRoleBindingName}, existingClusterRoleBinding)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ClusterRoleBinding %s: %w", clusterRoleBindingName, err)
		}

		// ClusterRoleBinding doesn't exist, create it
		log.Info("creating ClusterRoleBinding", "name", clusterRoleBindingName)
		if err := r.Create(context.TODO(), desiredClusterRoleBinding); err != nil {
			return fmt.Errorf("failed to create ClusterRoleBinding %s: %w", clusterRoleBindingName, err)
		}
		return nil
	}

	// ClusterRoleBinding exists, update if needed
	if !clusterRoleBindingNeedsUpdate(existingClusterRoleBinding, desiredClusterRoleBinding) {
		return nil
	}

	log.Info("updating ClusterRoleBinding", "name", clusterRoleBindingName)
	existingClusterRoleBinding.RoleRef = desiredClusterRoleBinding.RoleRef
	existingClusterRoleBinding.Subjects = desiredClusterRoleBinding.Subjects
	existingClusterRoleBinding.Labels = desiredClusterRoleBinding.Labels
	existingClusterRoleBinding.Annotations = desiredClusterRoleBinding.Annotations

	if err := r.Update(context.TODO(), existingClusterRoleBinding); err != nil {
		return fmt.Errorf("failed to update ClusterRoleBinding %s: %w", clusterRoleBindingName, err)
	}

	return nil
}

// clusterRoleBindingNeedsUpdate checks if a ClusterRoleBinding needs to be updated
func clusterRoleBindingNeedsUpdate(existing, desired *v1.ClusterRoleBinding) bool {
	// Check if RoleRef has changed
	if existing.RoleRef != desired.RoleRef {
		return true
	}

	// Check if Subjects have changed
	if len(existing.Subjects) != len(desired.Subjects) {
		return true
	}

	// TODO: Add more sophisticated comparison logic
	return false
}
