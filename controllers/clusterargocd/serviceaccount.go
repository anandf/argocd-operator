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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// reconcileServiceAccounts reconciles all ServiceAccounts for ClusterArgoCD components
func (r *ReconcileClusterArgoCD) reconcileServiceAccounts(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling ServiceAccounts for ClusterArgoCD", "name", clusterArgoCD.Name)

	targetNamespace := getTargetNamespace(clusterArgoCD)

	// Reconcile ServiceAccount for application-controller
	if err := r.reconcileServiceAccount(common.ArgoCDApplicationControllerComponent, targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile ServiceAccount for server
	if err := r.reconcileServiceAccount(common.ArgoCDServerComponent, targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile ServiceAccount for repo-server
	if err := r.reconcileServiceAccount(ArgoCDRepoServerComponent, targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile ServiceAccount for ApplicationSet controller (if enabled)
	if clusterArgoCD.Spec.ApplicationSet != nil {
		enabled := true
		if clusterArgoCD.Spec.ApplicationSet.Enabled != nil {
			enabled = *clusterArgoCD.Spec.ApplicationSet.Enabled
		}
		if enabled {
			if err := r.reconcileServiceAccount(common.ArgoCDApplicationSetControllerComponent, targetNamespace, clusterArgoCD); err != nil {
				return err
			}
		}
	}

	// Reconcile ServiceAccount for Notifications controller (if enabled)
	if clusterArgoCD.Spec.Notifications.Enabled {
		if err := r.reconcileServiceAccount(ArgoCDNotificationsControllerComponent, targetNamespace, clusterArgoCD); err != nil {
			return err
		}
	}

	// TODO: Add ServiceAccounts for other components (Dex, Redis, etc.) as needed

	return nil
}

// reconcileServiceAccount reconciles a single ServiceAccount
func (r *ReconcileClusterArgoCD) reconcileServiceAccount(componentName, targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	serviceAccountName := getServiceAccountName(componentName, clusterArgoCD)

	desiredServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceAccountName,
			Namespace:   targetNamespace,
			Labels:      getLabelsForClusterArgoCD(clusterArgoCD),
			Annotations: getAnnotationsForClusterArgoCD(clusterArgoCD),
		},
	}

	existingServiceAccount := &corev1.ServiceAccount{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: targetNamespace}, existingServiceAccount)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ServiceAccount %s/%s: %w", targetNamespace, serviceAccountName, err)
		}

		// ServiceAccount doesn't exist, create it
		log.Info("creating ServiceAccount", "namespace", targetNamespace, "name", serviceAccountName)
		if err := r.Create(context.TODO(), desiredServiceAccount); err != nil {
			return fmt.Errorf("failed to create ServiceAccount %s/%s: %w", targetNamespace, serviceAccountName, err)
		}
		return nil
	}

	// ServiceAccount exists, update labels/annotations if needed
	if !serviceAccountNeedsUpdate(existingServiceAccount, desiredServiceAccount) {
		return nil
	}

	log.Info("updating ServiceAccount", "namespace", targetNamespace, "name", serviceAccountName)
	existingServiceAccount.Labels = desiredServiceAccount.Labels
	existingServiceAccount.Annotations = desiredServiceAccount.Annotations

	if err := r.Update(context.TODO(), existingServiceAccount); err != nil {
		return fmt.Errorf("failed to update ServiceAccount %s/%s: %w", targetNamespace, serviceAccountName, err)
	}

	return nil
}

// serviceAccountNeedsUpdate checks if a ServiceAccount needs to be updated
func serviceAccountNeedsUpdate(existing, desired *corev1.ServiceAccount) bool {
	// Check if labels have changed
	if len(existing.Labels) != len(desired.Labels) {
		return true
	}
	for k, v := range desired.Labels {
		if existing.Labels[k] != v {
			return true
		}
	}

	// Check if annotations have changed
	if len(existing.Annotations) != len(desired.Annotations) {
		return true
	}
	for k, v := range desired.Annotations {
		if existing.Annotations[k] != v {
			return true
		}
	}

	return false
}
