# Proposal: Explicit Scoping with ClusterArgoCD CRD

## Summary

Introduce explicit scoping for ArgoCD instances by creating a separate `ClusterArgoCD` CRD for cluster-scoped instances, while keeping the existing `ArgoCD` CRD for namespace-scoped instances. This proposal aims to simplify the operator architecture and improve RBAC management.

## Motivation

**Current Issues:**
1. The single `ArgoCD` CRD handles both cluster-scoped and namespace-scoped configurations, leading to complexity
2. The `ARGOCD_CLUSTER_CONFIG_NAMESPACES` environment variable requires pre-configuration of allowed namespaces
3. Label `argocd.argoproj.io/managed-by-cluster-argocd` contains namespace. This was mainly done to identify an ArgoCD instance uniquely in a cluster.
4. Admin-specific features (argoCDAgent, sourceNamespaces) are available in all ArgoCD instances regardless of scope
5. RBAC requirements aren't clear from the CRD itself

**Goals:**
1. Explicit separation between cluster-scoped and namespace-scoped instances
2. Simplified operator configuration (remove ARGOCD_CLUSTER_CONFIG_NAMESPACES)
3. Unique identification of cluster-scoped instances
4. Clear RBAC requirements per instance type
5. Better security model with field-level restrictions

## Proposal

### 1. New ClusterArgoCD CRD

Create a new cluster-scoped CRD `ClusterArgoCD` with the following characteristics:

**Scope:** Cluster-scoped (not namespaced)
**Fields:** Inherits all fields from `ArgoCD`, including:
- `spec.sourceNamespaces` - Single field that defines namespaces where Applications, ApplicationSets, and NotificationConfigurations can be created
- `spec.argoCDAgent` - ArgoCD Agent configuration
- All other ArgoCD spec fields

**Note:** ClusterArgoCD uses a single `spec.sourceNamespaces` field to control cross-namespace access for all ArgoCD resources (Applications, ApplicationSets, and NotificationConfigurations). This simplifies configuration compared to having separate sourceNamespaces fields for each component. If different namespaces are needed for different resource types in the future, new top-level fields like `spec.appSetSourceNamespaces` and `spec.notificationSourceNamespaces` can be added.

**Naming:**
- Name must be unique cluster-wide
- Recommended naming: `cluster-argocd` (but users can choose any name)

### 2. Modified ArgoCD CRD

Keep existing `ArgoCD` CRD but **remove** admin-specific fields:
- Remove `spec.sourceNamespaces`
- Remove `spec.applicationSet.sourceNamespaces`
- Remove `spec.notifications.sourceNamespaces`
- Remove `spec.argoCDAgent`

**Scope:** Namespace-scoped (as it is today)
**Access:** Applications, ApplicationSets, and NotificationConfigurations can only be created in the same namespace as the ArgoCD instance. The deprecated `spec.applicationSet.sourceNamespaces` and `spec.notifications.sourceNamespaces` fields are no longer supported.

### 3. Label Changes

**Current:**
```yaml
argocd.argoproj.io/managed-by-cluster-argocd: <argocd-namespace>
```

**New:**
```yaml
argocd.argoproj.io/managed-by-cluster-argocd: <clusterargocd-name>
```

**Rationale:** ClusterArgoCD names are unique cluster-wide, providing true unique identification

### 4. RBAC Changes

#### For ClusterArgoCD (cluster-scoped instances):
- **Required RBAC:** ClusterRole and ClusterRoleBinding
- **Permissions:** Cluster-wide permissions to:
  - Manage resources in sourceNamespaces
  - Create/manage ArgoCD Applications across specified namespaces
  - Manage ArgoCD Agent components
  - Access cluster secrets globally

#### For ArgoCD (namespace-scoped instances):
- **Required RBAC:** Role and RoleBinding
- **Permissions:** Namespace-scoped permissions to:
  - Manage resources only within the ArgoCD instance namespace
  - Create/manage Applications only in same namespace
  - No cross-namespace access

### 5. Environment Variable Removal

**Remove:** `ARGOCD_CLUSTER_CONFIG_NAMESPACES`

**Rationale:**
- ClusterArgoCD is cluster-scoped, so no namespace restrictions needed
- Users with ClusterRole permissions can create ClusterArgoCD instances
- Standard Kubernetes RBAC controls access

## Implementation Plan

### Phase 1: API Changes

**Files to Create:**
- `api/v1beta1/clusterargocd_types.go` - New ClusterArgoCD CRD definition

**Files to Modify:**
- `api/v1beta1/argocd_types.go` - Remove sourceNamespaces and argoCDAgent fields
- `api/v1beta1/groupversion_info.go` - Register ClusterArgoCD type

**Steps:**
1. Create `ClusterArgoCDSpec` struct (copy from `ArgoCDSpec`)
2. Create `ClusterArgoCD` type with cluster scope annotation:
   ```go
   // +kubebuilder:resource:scope=Cluster
   type ClusterArgoCD struct {
       metav1.TypeMeta   `json:",inline"`
       metav1.ObjectMeta `json:"metadata,omitempty"`
       Spec   ClusterArgoCDSpec   `json:"spec,omitempty"`
       Status ArgoCDStatus `json:"status,omitempty"`
   }
   ```
3. Update ArgoCD types to deprecate/remove cluster-specific fields
4. Run `make manifests generate` to regenerate CRDs and DeepCopy methods

### Phase 2: Controller Changes

**New Controllers:**
- `controllers/clusterargocd/` - New controller package for ClusterArgoCD
  - Copy structure from `controllers/argocd/`
  - Modify to use cluster-scoped resources

**Modified Controllers:**
- `controllers/argocd/argocd_controller.go`
  - Remove sourceNamespace reconciliation logic for namespace-scoped instances
  - Update to only manage namespace-scoped resources

**Key Files to Modify:**

1. **cmd/main.go**
   - Register ClusterArgoCD controller
   - Remove ARGOCD_CLUSTER_CONFIG_NAMESPACES environment variable handling

2. **controllers/argocd/util.go**
   - Update label generation:
     ```go
     // For ClusterArgoCD instances
     func getClusterArgoCDManagedByLabel(clusterArgoCD *ClusterArgoCD) string {
         return clusterArgoCD.Name  // Not clusterArgoCD.Namespace
     }
     ```
   - Modify `getManagedSourceNamespaces()` to work with ClusterArgoCD
   - Update `removeUnmanagedSourceNamespaceResources()`

3. **controllers/argocd/role.go**
   - Add ClusterRole creation for ClusterArgoCD instances
   - Keep Role creation for ArgoCD instances
   - Update `reconcileRoleForApplicationSourceNamespaces()` for ClusterArgoCD

4. **controllers/argocd/rolebinding.go**
   - Add ClusterRoleBinding creation for ClusterArgoCD
   - Keep RoleBinding for ArgoCD instances

5. **controllers/argocd/secret.go**
   - Update cluster secret management to use ClusterArgoCD name in labels

6. **controllers/argocd/applicationset.go**
   - Update to use ClusterArgoCD for cross-namespace ApplicationSets

7. **controllers/argocd/notifications.go**
   - Update to use ClusterArgoCD for cross-namespace Notifications

8. **common/keys.go**
   - Update documentation for `ArgoCDManagedByClusterArgoCDLabel`

### Phase 3: Migration Strategy

**Backwards Compatibility:**

1. **Deprecation Period:**
   - Keep sourceNamespaces fields in ArgoCD v1beta1 with deprecation warnings
   - Document migration path in upgrade guide
   - Provide conversion webhook or migration tool

2. **Migration Tool:**
   Create a migration utility that:
   ```bash
   # Converts existing ArgoCD with sourceNamespaces to ClusterArgoCD
   argocd-operator migrate cluster-argocd \
     --argocd-name=<name> \
     --argocd-namespace=<namespace> \
     --cluster-argocd-name=<new-name>
   ```

3. **Migration Steps for Users:**
   - Export existing ArgoCD CR with sourceNamespaces
   - Create new ClusterArgoCD CR with admin fields
   - Create new namespace-scoped ArgoCD CR without admin fields
   - Verify applications still function
   - Delete old ArgoCD CR

### Phase 4: RBAC Updates

**Files to Modify:**
1. `config/rbac/role.yaml` - Update operator RBAC to include ClusterArgoCD
2. `controllers/argocd/role.go` - ClusterRole/Role creation logic
3. Documentation - Clear RBAC requirements

**New RBAC Requirements:**

```yaml
# For managing ClusterArgoCD instances
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: clusterargocd-manager
rules:
- apiGroups: ["argoproj.io"]
  resources: ["clusterargocds"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list", "watch", "update", "patch"]
# ... additional cluster-wide permissions

---
# For managing namespace-scoped ArgoCD instances
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: argocd-manager
  namespace: <namespace>
rules:
- apiGroups: ["argoproj.io"]
  resources: ["argocds"]
  verbs: ["*"]
# ... namespace-scoped permissions only
```

### Phase 5: Testing

**New Tests:**
1. **Unit Tests:**
   - `api/v1beta1/clusterargocd_types_test.go` - Type validation
   - `controllers/clusterargocd/*_test.go` - Controller logic tests

2. **E2E Tests:**
   - `tests/ginkgo/sequential/clusterargocd_test.go` - ClusterArgoCD lifecycle
   - `tests/ginkgo/sequential/clusterargocd_source_namespaces_test.go` - Cross-namespace apps
   - `tests/ginkgo/parallel/argocd_namespace_scoped_test.go` - Namespace-scoped instances
   - Migration test - Converting ArgoCD to ClusterArgoCD

**Modified Tests:**
- Update all tests using sourceNamespaces to use ClusterArgoCD
- Update all tests checking managed-by-cluster-argocd label

### Phase 6: Documentation

**New Documentation:**
1. `docs/usage/cluster-scoped-argocd.md` - Using ClusterArgoCD
2. `docs/migration/argocd-to-clusterargocd.md` - Migration guide
3. `docs/reference/clusterargocd.md` - ClusterArgoCD API reference

**Update Documentation:**
1. `docs/usage/basics.md` - Clarify scope types
2. `docs/usage/apps-in-any-namespace.md` - Update for ClusterArgoCD
3. `docs/usage/appsets-in-any-namespace.md` - Update for ClusterArgoCD
4. `docs/usage/notifications-in-any-namespace.md` - Update for ClusterArgoCD
5. `README.md` - Mention both CRD types

## Implementation Checklist

### API Changes
- [ ] Create `api/v1beta1/clusterargocd_types.go`
- [ ] Add ClusterArgoCDSpec with all admin fields
- [ ] Add ClusterArgoCD and ClusterArgoCDList types
- [ ] Register in `groupversion_info.go`
- [ ] Deprecate sourceNamespaces fields in ArgoCD
- [ ] Run `make manifests generate`
- [ ] Update CRD with scope=Cluster annotation

### Controller Implementation
- [ ] Create `controllers/clusterargocd/` package
- [ ] Copy and adapt reconciler from argocd controller
- [ ] Update label management (remove namespace from label value)
- [ ] Implement ClusterRole/ClusterRoleBinding creation
- [ ] Update sourceNamespace reconciliation for ClusterArgoCD
- [ ] Register controller in `cmd/main.go`
- [ ] Remove ARGOCD_CLUSTER_CONFIG_NAMESPACES handling

### RBAC Updates
- [ ] Add ClusterArgoCD permissions to operator ClusterRole
- [ ] Update role.go to create ClusterRoles for ClusterArgoCD instances
- [ ] Update rolebinding.go for ClusterRoleBindings
- [ ] Document RBAC requirements in proposal

### Label and Resource Updates
- [ ] Update label value from namespace to ClusterArgoCD name
- [ ] Update secret management to use new label format
- [ ] Update applicationset controller integration
- [ ] Update notifications controller integration
- [ ] Update cluster secret reconciliation

### Testing
- [ ] Unit tests for ClusterArgoCD types
- [ ] Unit tests for ClusterArgoCD controller
- [ ] E2E test: Create ClusterArgoCD instance
- [ ] E2E test: Cross-namespace Applications with ClusterArgoCD
- [ ] E2E test: Namespace-scoped ArgoCD restrictions
- [ ] E2E test: Label validation
- [ ] E2E test: RBAC enforcement
- [ ] Migration test: ArgoCD to ClusterArgoCD conversion

### Documentation
- [ ] ClusterArgoCD usage guide
- [ ] Migration guide with examples
- [ ] API reference documentation
- [ ] Update existing docs for scope clarification
- [ ] Update CLAUDE.md with new architecture
- [ ] Release notes with breaking changes

### Migration Support
- [ ] Add deprecation warnings to sourceNamespaces fields
- [ ] Create migration script/tool
- [ ] Add conversion webhook (optional)
- [ ] Create example migration manifests
- [ ] Document upgrade path

## Risks and Mitigations

### Risk 1: Breaking Change for Existing Users
**Mitigation:**
- Maintain backwards compatibility for 2-3 releases
- Provide clear migration documentation
- Create automated migration tool
- Add deprecation warnings early

### Risk 2: Complex Migration Path
**Mitigation:**
- Step-by-step migration guide with examples
- Migration validation tool
- Support for running both old and new style simultaneously

### Risk 3: Label Update Disruption
**Mitigation:**
- Implement label update in controller reconciliation
- Ensure applications continue working during transition
- Test thoroughly with existing applications

### Risk 4: RBAC Permission Issues
**Mitigation:**
- Clear documentation of required permissions
- Pre-flight validation in controller
- Helpful error messages for permission issues

## Future Enhancements

1. **Multi-Cluster Support:** ClusterArgoCD instances managing multiple Kubernetes clusters
2. **Hierarchical ArgoCD:** ClusterArgoCD managing multiple namespace-scoped ArgoCD instances
3. **Dynamic Namespace Discovery:** Auto-discovery of allowed namespaces based on labels
4. **Advanced RBAC:** Fine-grained permissions per sourceNamespace

## References

- Current implementation: `api/v1beta1/argocd_types.go`
- Source namespace handling: `controllers/argocd/util.go`
- Label definitions: `common/keys.go`
- Related tests: `tests/ginkgo/sequential/1-036_validate_role_rolebinding_for_source_namespace_test.go`
