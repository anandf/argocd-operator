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
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ArgoCDDeletionFinalizer is the finalizer for ClusterArgoCD
	ArgoCDDeletionFinalizer = "argoproj.io/finalizer"
)

// addDeletionFinalizer adds the deletion finalizer to the ClusterArgoCD instance
func (r *ReconcileClusterArgoCD) addDeletionFinalizer(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("adding deletion finalizer", "name", clusterArgoCD.Name)

	clusterArgoCD.Finalizers = append(clusterArgoCD.Finalizers, ArgoCDDeletionFinalizer)
	if err := r.Update(context.TODO(), clusterArgoCD); err != nil {
		return fmt.Errorf("failed to add deletion finalizer to ClusterArgoCD %s: %w", clusterArgoCD.Name, err)
	}

	return nil
}

// removeDeletionFinalizer removes the deletion finalizer from the ClusterArgoCD instance
func (r *ReconcileClusterArgoCD) removeDeletionFinalizer(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("removing deletion finalizer", "name", clusterArgoCD.Name)

	// Remove the finalizer from the slice
	finalizers := []string{}
	for _, finalizer := range clusterArgoCD.Finalizers {
		if finalizer != ArgoCDDeletionFinalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	clusterArgoCD.Finalizers = finalizers

	if err := r.Update(context.TODO(), clusterArgoCD); err != nil {
		return fmt.Errorf("failed to remove deletion finalizer from ClusterArgoCD %s: %w", clusterArgoCD.Name, err)
	}

	return nil
}

// deleteClusterResources deletes all cluster-scoped resources created by ClusterArgoCD
func (r *ReconcileClusterArgoCD) deleteClusterResources(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("deleting cluster resources for ClusterArgoCD", "name", clusterArgoCD.Name)

	// Create label selector for resources owned by this ClusterArgoCD
	selector := labels.SelectorFromSet(map[string]string{
		common.ArgoCDKeyName: clusterArgoCD.Name,
	})

	// Delete ClusterRoles
	clusterRoleList := &v1.ClusterRoleList{}
	listOpts := &client.ListOptions{
		LabelSelector: selector,
	}

	if err := r.List(context.TODO(), clusterRoleList, listOpts); err != nil {
		log.Error(err, "failed to list ClusterRoles for deletion")
		return err
	}

	for _, clusterRole := range clusterRoleList.Items {
		log.Info("deleting ClusterRole", "name", clusterRole.Name)
		if err := r.Delete(context.TODO(), &clusterRole); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "failed to delete ClusterRole", "name", clusterRole.Name)
			return err
		}
	}

	// Delete ClusterRoleBindings
	clusterRoleBindingList := &v1.ClusterRoleBindingList{}
	if err := r.List(context.TODO(), clusterRoleBindingList, listOpts); err != nil {
		log.Error(err, "failed to list ClusterRoleBindings for deletion")
		return err
	}

	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		log.Info("deleting ClusterRoleBinding", "name", clusterRoleBinding.Name)
		if err := r.Delete(context.TODO(), &clusterRoleBinding); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "failed to delete ClusterRoleBinding", "name", clusterRoleBinding.Name)
			return err
		}
	}

	log.Info("successfully deleted all cluster resources", "name", clusterArgoCD.Name)
	return nil
}

// cleanupAllSourceNamespaces removes labels and resources from all source namespaces
func (r *ReconcileClusterArgoCD) cleanupAllSourceNamespaces(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("cleaning up all source namespaces", "name", clusterArgoCD.Name)

	// This will remove labels from all namespaces managed by this ClusterArgoCD
	// The cleanupUnmanagedSourceNamespaces function with an empty spec will clean up everything
	emptyClusterArgoCD := &argoproj.ClusterArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterArgoCD.Name,
		},
		Spec: argoproj.ClusterArgoCDSpec{
			// Empty spec means no source namespaces - cleanup all
			SourceNamespaces: []string{},
		},
	}

	if err := r.cleanupUnmanagedSourceNamespaces(emptyClusterArgoCD); err != nil {
		return fmt.Errorf("failed to cleanup source namespaces: %w", err)
	}

	return nil
}

// cleanupClusterInstanceTokenTimers removes all token renewal timers for this ClusterArgoCD instance
func (r *ReconcileClusterArgoCD) cleanupClusterInstanceTokenTimers(clusterArgoCDName string) {
	if r.LocalUsers == nil || r.LocalUsers.TokenRenewalTimers == nil {
		return
	}

	log.Info("cleaning up token renewal timers", "clusterArgoCD", clusterArgoCDName)

	// Find and delete all timers for this ClusterArgoCD instance
	// Timer keys are in format "namespace/user-name" but for ClusterArgoCD we use "clusterargocd-name/user-name"
	// Note: The actual timer cleanup will be handled by the shared LocalUsersInfo structure
	// We just need to identify and remove the entries
	keysToDelete := []string{}
	for key := range r.LocalUsers.TokenRenewalTimers {
		// Check if the key starts with the ClusterArgoCD name
		if len(key) > len(clusterArgoCDName) && key[:len(clusterArgoCDName)] == clusterArgoCDName {
			keysToDelete = append(keysToDelete, key)
		}
	}

	// Delete the timer entries
	// The lock is handled internally by the shared structure
	for _, key := range keysToDelete {
		delete(r.LocalUsers.TokenRenewalTimers, key)
	}
}
