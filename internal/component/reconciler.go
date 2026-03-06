package component

import (
	"context"
	"fmt"
	"reflect"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

var reconcilerLog = logs.Log.WithName("component-reconciler")

// ReconcileResource applies decorators, sets owner references, and creates or updates
// a Kubernetes resource. This is the shared pattern used by all component controllers.
func ReconcileResource(ctx context.Context, c client.Client, scheme *runtime.Scheme,
	owner *argoproj.ArgoCD, desired client.Object,
	decorators *decorator.DecoratorManager) error {

	// Apply decorators if provided
	if decorators != nil {
		if runtimeObj, ok := desired.(runtime.Object); ok {
			if err := decorators.Decorate(runtimeObj); err != nil {
				return fmt.Errorf("failed to apply decorators: %w", err)
			}
		}
	}

	// Set owner reference for namespaced resources in the same namespace
	if desired.GetNamespace() == owner.Namespace && desired.GetNamespace() != "" {
		if err := controllerutil.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}

	// Create or update the resource
	kind := resourceKind(desired)
	existing := desired.DeepCopyObject().(client.Object)
	result, err := controllerutil.CreateOrUpdate(ctx, c, existing, func() error {
		return mergeResource(existing, desired)
	})
	if err != nil {
		return fmt.Errorf("failed to create or update %s %s/%s: %w",
			kind, desired.GetNamespace(), desired.GetName(), err)
	}

	reconcilerLog.Info("reconciled resource",
		"kind", kind,
		"namespace", desired.GetNamespace(),
		"name", desired.GetName(),
		"result", result)

	return nil
}

// resourceKind returns the kind of a client.Object. It first checks the GVK
// (set on unstructured objects), then falls back to the Go type name (for typed
// objects where GVK is stripped after conversion).
func resourceKind(obj client.Object) string {
	if gvk := obj.GetObjectKind().GroupVersionKind(); gvk.Kind != "" {
		return gvk.Kind
	}
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// mergeResource copies the desired spec into the existing object, preserving metadata
// that Kubernetes manages (resourceVersion, uid, etc.).
func mergeResource(existing, desired client.Object) error {
	// Merge labels
	existingLabels := existing.GetLabels()
	if existingLabels == nil {
		existingLabels = make(map[string]string)
	}
	for k, v := range desired.GetLabels() {
		existingLabels[k] = v
	}
	existing.SetLabels(existingLabels)

	// Merge annotations
	existingAnnotations := existing.GetAnnotations()
	if existingAnnotations == nil {
		existingAnnotations = make(map[string]string)
	}
	for k, v := range desired.GetAnnotations() {
		existingAnnotations[k] = v
	}
	existing.SetAnnotations(existingAnnotations)

	// Set owner references from desired
	existing.SetOwnerReferences(desired.GetOwnerReferences())

	// Type-specific spec merging
	switch e := existing.(type) {
	case *appsv1.Deployment:
		d, ok := desired.(*appsv1.Deployment)
		if !ok {
			return fmt.Errorf("type mismatch: existing is Deployment but desired is %T", desired)
		}
		e.Spec = d.Spec
	case *appsv1.StatefulSet:
		d, ok := desired.(*appsv1.StatefulSet)
		if !ok {
			return fmt.Errorf("type mismatch: existing is StatefulSet but desired is %T", desired)
		}
		e.Spec = d.Spec
	case *corev1.Service:
		d, ok := desired.(*corev1.Service)
		if !ok {
			return fmt.Errorf("type mismatch: existing is Service but desired is %T", desired)
		}
		// Preserve ClusterIP on update
		clusterIP := e.Spec.ClusterIP
		e.Spec = d.Spec
		if clusterIP != "" {
			e.Spec.ClusterIP = clusterIP
		}
	case *corev1.ServiceAccount:
		// ServiceAccounts don't have a spec to merge; labels/annotations are sufficient
	case *corev1.ConfigMap:
		d, ok := desired.(*corev1.ConfigMap)
		if !ok {
			return fmt.Errorf("type mismatch: existing is ConfigMap but desired is %T", desired)
		}
		e.Data = d.Data
		e.BinaryData = d.BinaryData
	case *rbacv1.Role:
		d, ok := desired.(*rbacv1.Role)
		if !ok {
			return fmt.Errorf("type mismatch: existing is Role but desired is %T", desired)
		}
		e.Rules = d.Rules
	case *rbacv1.RoleBinding:
		d, ok := desired.(*rbacv1.RoleBinding)
		if !ok {
			return fmt.Errorf("type mismatch: existing is RoleBinding but desired is %T", desired)
		}
		e.RoleRef = d.RoleRef
		e.Subjects = d.Subjects
	case *rbacv1.ClusterRole:
		d, ok := desired.(*rbacv1.ClusterRole)
		if !ok {
			return fmt.Errorf("type mismatch: existing is ClusterRole but desired is %T", desired)
		}
		e.Rules = d.Rules
		e.AggregationRule = d.AggregationRule
	case *rbacv1.ClusterRoleBinding:
		d, ok := desired.(*rbacv1.ClusterRoleBinding)
		if !ok {
			return fmt.Errorf("type mismatch: existing is ClusterRoleBinding but desired is %T", desired)
		}
		e.RoleRef = d.RoleRef
		e.Subjects = d.Subjects
	case *networkingv1.Ingress:
		d, ok := desired.(*networkingv1.Ingress)
		if !ok {
			return fmt.Errorf("type mismatch: existing is Ingress but desired is %T", desired)
		}
		e.Spec = d.Spec
	default:
		return fmt.Errorf("unsupported resource type for merge: %T", existing)
	}

	return nil
}

// DeleteResourceIfExists deletes a Kubernetes resource if it exists.
// Returns nil if the resource was deleted or did not exist.
func DeleteResourceIfExists(ctx context.Context, c client.Client, name, namespace string, obj client.Object) error {
	kind := resourceKind(obj)
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get %s %s/%s for cleanup: %w", kind, namespace, name, err)
	}

	if err := c.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete %s %s/%s: %w", kind, namespace, name, err)
	}

	reconcilerLog.Info("deleted resource during cleanup",
		"kind", kind,
		"namespace", namespace,
		"name", name)

	return nil
}

// ReconcileUnstructuredResource creates or updates an unstructured Kubernetes resource.
// This is used for resources like OpenShift Routes where we don't want to import the
// typed API (routev1) into the component layer.
func ReconcileUnstructuredResource(ctx context.Context, c client.Client, scheme *runtime.Scheme,
	owner *argoproj.ArgoCD, desired *unstructured.Unstructured) error {

	// Set owner reference for namespaced resources in the same namespace
	if desired.GetNamespace() == owner.Namespace && desired.GetNamespace() != "" {
		if err := controllerutil.SetControllerReference(owner, desired, scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	}

	kind := desired.GetKind()
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())

	key := types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}
	err := c.Get(ctx, key, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create %s %s/%s: %w",
					kind, desired.GetNamespace(), desired.GetName(), err)
			}
			reconcilerLog.Info("created unstructured resource",
				"kind", kind,
				"namespace", desired.GetNamespace(),
				"name", desired.GetName())
			return nil
		}
		return fmt.Errorf("failed to get %s %s/%s: %w",
			kind, desired.GetNamespace(), desired.GetName(), err)
	}

	// Merge labels
	existingLabels := existing.GetLabels()
	if existingLabels == nil {
		existingLabels = make(map[string]string)
	}
	for k, v := range desired.GetLabels() {
		existingLabels[k] = v
	}
	existing.SetLabels(existingLabels)

	// Merge annotations
	existingAnnotations := existing.GetAnnotations()
	if existingAnnotations == nil {
		existingAnnotations = make(map[string]string)
	}
	for k, v := range desired.GetAnnotations() {
		existingAnnotations[k] = v
	}
	existing.SetAnnotations(existingAnnotations)

	// Set owner references from desired
	existing.SetOwnerReferences(desired.GetOwnerReferences())

	// Copy spec from desired to existing
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")
	if desiredSpec != nil {
		if err := unstructured.SetNestedMap(existing.Object, desiredSpec, "spec"); err != nil {
			return fmt.Errorf("failed to set spec on %s %s/%s: %w",
				kind, desired.GetNamespace(), desired.GetName(), err)
		}
	}

	if err := c.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update %s %s/%s: %w",
			kind, desired.GetNamespace(), desired.GetName(), err)
	}

	reconcilerLog.Info("updated unstructured resource",
		"kind", kind,
		"namespace", desired.GetNamespace(),
		"name", desired.GetName())

	return nil
}

// DeleteUnstructuredResourceIfExists deletes an unstructured resource if it exists.
func DeleteUnstructuredResourceIfExists(ctx context.Context, c client.Client, name, namespace string, gvk schema.GroupVersionKind) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get %s %s/%s for cleanup: %w", gvk.Kind, namespace, name, err)
	}

	if err := c.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete %s %s/%s: %w", gvk.Kind, namespace, name, err)
	}

	reconcilerLog.Info("deleted unstructured resource during cleanup",
		"kind", gvk.Kind,
		"namespace", namespace,
		"name", name)

	return nil
}
