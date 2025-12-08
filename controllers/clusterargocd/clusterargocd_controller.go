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
	"time"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/controllers/argocd"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logr "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// blank assignment to verify that ReconcileClusterArgoCD implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileClusterArgoCD{}

// ReconcileClusterArgoCD reconciles a ClusterArgoCD object
type ReconcileClusterArgoCD struct {
	client.Client
	Scheme            *runtime.Scheme
	ManagedNamespaces *corev1.NamespaceList
	// Stores a list of ApplicationSourceNamespaces as keys
	ManagedSourceNamespaces map[string]string
	// Stores a list of ApplicationSetSourceNamespaces as keys
	ManagedApplicationSetSourceNamespaces map[string]string

	// Stores a list of NotificationsSourceNamespaces as keys
	ManagedNotificationsSourceNamespaces map[string]string

	// Stores label selector used to reconcile a subset of ClusterArgoCD
	LabelSelector string

	K8sClient kubernetes.Interface
	// Reuse LocalUsersInfo from argocd package
	LocalUsers *argocd.LocalUsersInfo
}

var log = logr.Log.WithName("controller_clusterargocd")

// Map to keep track of running ClusterArgoCD instances using their names as key and phase as value
// This map will be used for the performance metrics purposes
var ActiveClusterInstanceMap = make(map[string]string)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=*
//+kubebuilder:rbac:groups="",resources=configmaps;endpoints;events;persistentvolumeclaims;pods;namespaces;secrets;serviceaccounts;services;services/finalizers,verbs=*
//+kubebuilder:rbac:groups=apps,resources=deployments;replicasets;daemonsets;statefulsets,verbs=*
//+kubebuilder:rbac:groups=argoproj.io,resources=clusterargocds;clusterargocds/finalizers;clusterargocds/status,verbs=*
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=*
//+kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=*
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=*
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=create;delete;get;list;patch;update;watch;
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses;prometheusrules;servicemonitors,verbs=*
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=*
//+kubebuilder:rbac:groups=argoproj.io,resources=applications;appprojects,verbs=*
//+kubebuilder:rbac:groups="",resources=pods;pods/log,verbs=get
//+kubebuilder:rbac:groups=template.openshift.io,resources=templates;templateinstances;templateconfigs,verbs=*
//+kubebuilder:rbac:groups="oauth.openshift.io",resources=oauthclients,verbs=get;list;watch;create;delete;patch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ReconcileClusterArgoCD) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {

	result, clusterArgoCD, status, err := r.internalReconcile(ctx, request)

	message := ""
	if err != nil {
		message = err.Error()
		status.Phase = "Failed" // Any error should reset phase back to Failed
	}

	log.Info("reconciling status")
	if reconcileStatusErr := r.reconcileStatus(clusterArgoCD, status); reconcileStatusErr != nil {
		log.Error(reconcileStatusErr, "Unable to reconcile status")
		status.Phase = "Failed"
		message = "unable to reconcile ClusterArgoCD CR .status field"
	}

	if updateStatusErr := updateStatusAndConditionsOfClusterArgoCD(ctx, createCondition(message), clusterArgoCD, status, r.Client); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status of ClusterArgoCD")
		return reconcile.Result{}, updateStatusErr
	}

	return result, err
}

func (r *ReconcileClusterArgoCD) internalReconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, *argoproj.ClusterArgoCD, *argoproj.ArgoCDStatus, error) {

	argoCDStatus := &argoproj.ArgoCDStatus{} // Start with a blank canvas

	reconcileStartTS := time.Now()
	defer func() {
		argocd.ReconcileTime.WithLabelValues("cluster-scoped").Observe(time.Since(reconcileStartTS).Seconds())
	}()

	reqLogger := logr.FromContext(ctx, "name", request.Name)
	reqLogger.Info("Reconciling ClusterArgoCD")

	clusterArgoCD := &argoproj.ClusterArgoCD{}
	// ClusterArgoCD is cluster-scoped, so we use types.NamespacedName with empty namespace
	err := r.Get(ctx, types.NamespacedName{Name: request.Name}, clusterArgoCD)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, clusterArgoCD, argoCDStatus, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, clusterArgoCD, argoCDStatus, err
	}

	// Redis TLS Checksum and Repo Server TLS Checksum should be preserved between reconcile calls
	argoCDStatus.RepoTLSChecksum = clusterArgoCD.Status.RepoTLSChecksum
	argoCDStatus.RedisTLSChecksum = clusterArgoCD.Status.RedisTLSChecksum

	// If the number of notification replicas is greater than 1, display a warning.
	if clusterArgoCD.Spec.Notifications.Replicas != nil && *clusterArgoCD.Spec.Notifications.Replicas > 1 {
		reqLogger.Info("WARNING: Argo CD Notification controller does not support multiple replicas. Notification replicas cannot be greater than 1.")
	}

	// Fetch labelSelector from r.LabelSelector (command-line option)
	labelSelector, err := labels.Parse(r.LabelSelector)
	if err != nil {
		message := fmt.Sprintf("error parsing the labelSelector '%s'.", labelSelector)
		reqLogger.Error(err, message)
		return reconcile.Result{}, clusterArgoCD, argoCDStatus, fmt.Errorf("%s error: %w", message, err)
	}

	// Match the value of labelSelector from ReconcileClusterArgoCD to labels from the clusterArgoCD instance
	if !labelSelector.Matches(labels.Set(clusterArgoCD.Labels)) {
		reqLogger.Error(nil, fmt.Sprintf("the ClusterArgoCD instance '%s' does not match the label selector '%s' and skipping for reconciliation", request.Name, r.LabelSelector))
		return reconcile.Result{}, clusterArgoCD, argoCDStatus, fmt.Errorf("the ClusterArgoCD instance '%s' does not match the label selector '%s' and skipping for reconciliation", request.Name, r.LabelSelector)
	}

	newPhase := clusterArgoCD.Status.Phase
	// Track ClusterArgoCD instance in the active instances map
	if _, ok := ActiveClusterInstanceMap[request.Name]; !ok {
		if newPhase != "" {
			ActiveClusterInstanceMap[request.Name] = newPhase
			argocd.ActiveInstancesByPhase.WithLabelValues(newPhase).Inc()
			argocd.ActiveInstancesTotal.Inc()
		}
	} else {
		// If phase changed, update metrics
		if oldPhase := ActiveClusterInstanceMap[request.Name]; oldPhase != newPhase {
			ActiveClusterInstanceMap[request.Name] = newPhase
			argocd.ActiveInstancesByPhase.WithLabelValues(newPhase).Inc()
			argocd.ActiveInstancesByPhase.WithLabelValues(oldPhase).Dec()
		}
	}

	argocd.ActiveInstanceReconciliationCount.WithLabelValues(request.Name).Inc()

	// Handle deletion
	if clusterArgoCD.GetDeletionTimestamp() != nil {
		argoCDStatus.Phase = "Unknown" // Set to Unknown since we are in the process of deleting ClusterArgoCD CR

		// Remove from active instances map and decrement metrics
		delete(ActiveClusterInstanceMap, request.Name)
		argocd.ActiveInstancesByPhase.WithLabelValues(newPhase).Dec()
		argocd.ActiveInstancesTotal.Dec()
		argocd.ActiveInstanceReconciliationCount.DeleteLabelValues(request.Name)

		// Remove any local user token renewal timers for this instance
		r.cleanupClusterInstanceTokenTimers(request.Name)

		if clusterArgoCD.IsDeletionFinalizerPresent() {
			// Delete cluster resources (ClusterRoles, ClusterRoleBindings, etc.)
			if err := r.deleteClusterResources(clusterArgoCD); err != nil {
				return reconcile.Result{}, clusterArgoCD, argoCDStatus, fmt.Errorf("failed to delete ClusterResources: %w", err)
			}

			// Clean up source namespace labels and resources
			if err := r.cleanupAllSourceNamespaces(clusterArgoCD); err != nil {
				return reconcile.Result{}, clusterArgoCD, argoCDStatus, fmt.Errorf("failed to cleanup source namespaces: %w", err)
			}

			// Remove finalizer
			if err := r.removeDeletionFinalizer(clusterArgoCD); err != nil {
				return reconcile.Result{}, clusterArgoCD, argoCDStatus, err
			}
		}

		return reconcile.Result{}, clusterArgoCD, argoCDStatus, nil
	}

	// Add finalizer if not present
	if !clusterArgoCD.IsDeletionFinalizerPresent() {
		if err := r.addDeletionFinalizer(clusterArgoCD); err != nil {
			return reconcile.Result{}, clusterArgoCD, argoCDStatus, err
		}
	}

	// Reconcile source namespaces
	log.Info("reconciling source namespaces")
	if err = r.reconcileSourceNamespaces(clusterArgoCD); err != nil {
		return reconcile.Result{}, clusterArgoCD, argoCDStatus, err
	}

	// Reconcile all ClusterArgoCD resources
	log.Info("reconciling ClusterArgoCD resources")
	if err := r.reconcileResources(clusterArgoCD, argoCDStatus); err != nil {
		// Error reconciling ClusterArgoCD sub-resources - requeue the request.
		return reconcile.Result{}, clusterArgoCD, argoCDStatus, err
	}

	// Return and don't requeue
	return reconcile.Result{}, clusterArgoCD, argoCDStatus, nil
}

// reconcileStatus updates the status of the ClusterArgoCD resource
func (r *ReconcileClusterArgoCD) reconcileStatus(clusterArgoCD *argoproj.ClusterArgoCD, status *argoproj.ArgoCDStatus) error {
	// TODO: Implement status reconciliation similar to ArgoCD controller
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileClusterArgoCD) SetupWithManager(mgr ctrl.Manager) error {
	bldr := ctrl.NewControllerManagedBy(mgr).
		For(&argoproj.ClusterArgoCD{})
	// TODO: Add watches for owned resources similar to ArgoCD controller
	return bldr.Complete(r)
}

// Helper functions

func createCondition(message string) string {
	return message
}

func updateStatusAndConditionsOfClusterArgoCD(ctx context.Context, condition string, clusterArgoCD *argoproj.ClusterArgoCD, status *argoproj.ArgoCDStatus, c client.Client) error {
	// Update the ClusterArgoCD status
	clusterArgoCD.Status = *status
	if err := c.Status().Update(ctx, clusterArgoCD); err != nil {
		return err
	}
	return nil
}
