package component

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj/argo-cd/v2/util/glob"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Returns the name of the role/rolebinding for the source namespaces for applicationset-controller in the format of "argocdName-argocdNamespace-applicationset"
func getResourceNameForApplicationSetSourceNamespaces(cr *argoproj.ArgoCD) string {
	return fmt.Sprintf("%s-%s-applicationset", cr.Name, cr.Namespace)
}

// identifyDeploymentDifference is a simple comparison of the contents of two
// deployments, returning "" if they are the same, otherwise returning the name
// of the field that changed.
func identifyDeploymentDifference(x appsv1.Deployment, y appsv1.Deployment) string {

	xPodSpec := x.Spec.Template.Spec
	yPodSpec := y.Spec.Template.Spec

	if !reflect.DeepEqual(xPodSpec.Containers, yPodSpec.Containers) {
		return ".Spec.Template.Spec.Containers"
	}

	if !reflect.DeepEqual(xPodSpec.Volumes, yPodSpec.Volumes) {
		return ".Spec.Template.Spec.Volumes"
	}

	if xPodSpec.ServiceAccountName != yPodSpec.ServiceAccountName {
		return "ServiceAccountName"
	}

	if !reflect.DeepEqual(x.Labels, y.Labels) {
		return "Labels"
	}

	if !reflect.DeepEqual(x.Spec.Template.Labels, y.Spec.Template.Labels) {
		return ".Spec.Template.Labels"
	}

	if !reflect.DeepEqual(x.Spec.Selector, y.Spec.Selector) {
		return ".Spec.Selector"
	}

	if !reflect.DeepEqual(xPodSpec.NodeSelector, yPodSpec.NodeSelector) {
		return "Spec.Template.Spec.NodeSelector"
	}

	if !reflect.DeepEqual(xPodSpec.Tolerations, yPodSpec.Tolerations) {
		return "Spec.Template.Spec.Tolerations"
	}

	if !reflect.DeepEqual(xPodSpec.Containers[0].SecurityContext, yPodSpec.Containers[0].SecurityContext) {
		return "Spec.Template.Spec..Containers[0].SecurityContext"
	}

	if !reflect.DeepEqual(x.Spec.Template.Annotations, y.Spec.Template.Annotations) {
		return ".Spec.Template.Annotations"
	}

	return ""
}

func getApplicationSetContainerImage(cr *argoproj.ArgoCD) string {

	defaultImg, defaultTag := false, false
	img := cr.Spec.ApplicationSet.Image
	if img == "" {
		img = cr.Spec.Image
		if img == "" {
			img = common.ArgoCDDefaultArgoImage
			defaultImg = true
		}
	}

	tag := cr.Spec.ApplicationSet.Version
	if tag == "" {
		tag = cr.Spec.Version
		if tag == "" {
			tag = common.ArgoCDDefaultArgoVersion
			defaultTag = true
		}
	}

	// If an env var is specified then use that, but don't override the spec values (if they are present)
	if e := os.Getenv(common.ArgoCDImageEnvName); e != "" && (defaultTag && defaultImg) {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// getApplicationSetResources will return the ResourceRequirements for the Application Sets container.
func getApplicationSetResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}

	// Allow override of resource requirements from CR
	if cr.Spec.ApplicationSet.Resources != nil {
		resources = *cr.Spec.ApplicationSet.Resources
	}

	return resources
}

func setAppSetLabels(obj *metav1.ObjectMeta) {
	obj.Labels["app.kubernetes.io/name"] = "argocd-applicationset-controller"
	obj.Labels["app.kubernetes.io/part-of"] = "argocd"
	obj.Labels["app.kubernetes.io/component"] = "controller"
}

// newServiceAccountWithName creates a new ServiceAccount with the given name for the given ArgCD.
func newServiceAccountWithName(name string, cr *argoproj.ArgoCD) *corev1.ServiceAccount {
	sa := newServiceAccount(cr)
	sa.ObjectMeta.Name = getServiceAccountName(cr.Name, name)

	lbls := sa.ObjectMeta.Labels
	lbls[common.ArgoCDKeyName] = name
	sa.ObjectMeta.Labels = lbls

	return sa
}

func getServiceAccountName(crName, name string) string {
	return fmt.Sprintf("%s-%s", crName, name)
}

// newServiceAccount returns a new ServiceAccount instance.
func newServiceAccount(cr *argoproj.ArgoCD) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    argoutil.LabelsForCluster(cr),
		},
	}
}

// newDeploymentWithSuffix returns a new Deployment instance for the given ArgoCD using the given suffix.
func newDeploymentWithSuffix(suffix string, component string, cr *argoproj.ArgoCD) *appsv1.Deployment {
	return newDeploymentWithName(fmt.Sprintf("%s-%s", cr.Name, suffix), component, cr)
}

// newDeploymentWithName returns a new Deployment instance for the given ArgoCD using the given name.
func newDeploymentWithName(name string, component string, cr *argoproj.ArgoCD) *appsv1.Deployment {
	deploy := newDeployment(cr)
	deploy.ObjectMeta.Name = name

	lbls := deploy.ObjectMeta.Labels
	lbls[common.ArgoCDKeyName] = name
	lbls[common.ArgoCDKeyComponent] = component
	deploy.ObjectMeta.Labels = lbls

	deploy.Spec = appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				common.ArgoCDKeyName: name,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					common.ArgoCDKeyName: name,
				},
				Annotations: make(map[string]string),
			},
			Spec: corev1.PodSpec{
				NodeSelector: common.DefaultNodeSelector(),
			},
		},
	}

	if cr.Spec.NodePlacement != nil {
		deploy.Spec.Template.Spec.NodeSelector = argoutil.AppendStringMap(deploy.Spec.Template.Spec.NodeSelector, cr.Spec.NodePlacement.NodeSelector)
		deploy.Spec.Template.Spec.Tolerations = cr.Spec.NodePlacement.Tolerations
	}
	return deploy
}

// newDeployment returns a new Deployment instance for the given ArgoCD.
func newDeployment(cr *argoproj.ArgoCD) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    argoutil.LabelsForCluster(cr),
		},
	}
}

func newClusterRole(name string, rules []v1.PolicyRule, cr *argoproj.ArgoCD) *v1.ClusterRole {
	return &v1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:        GenerateUniqueResourceName(name, cr),
			Labels:      argoutil.LabelsForCluster(cr),
			Annotations: argoutil.AnnotationsForCluster(cr),
		},
		Rules: rules,
	}
}

// GenerateUniqueResourceName generates unique names for cluster scoped resources
func GenerateUniqueResourceName(argoComponentName string, cr *argoproj.ArgoCD) string {
	return cr.Name + "-" + cr.Namespace + "-" + argoComponentName
}

// getRepoServerAddress will return the Argo CD repo server address.
func getRepoServerAddress(cr *argoproj.ArgoCD) string {
	if cr.Spec.Repo.IsRemote() {
		return *cr.Spec.Repo.Remote
	}
	return fqdnServiceRef("repo-server", common.ArgoCDDefaultRepoServerPort, cr)
}

// fqdnServiceRef will return the FQDN referencing a specific service name, as set up by the operator, with the
// given port.
func fqdnServiceRef(service string, port int, cr *argoproj.ArgoCD) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", nameWithSuffix(service, cr), cr.Namespace, port)
}

// nameWithSuffix will return a name based on the given ArgoCD. The given suffix is appended to the generated name.
// Example: Given an ArgoCD with the name "example-argocd", providing the suffix "foo" would result in the value of
// "example-argocd-foo" being returned.
func nameWithSuffix(suffix string, cr *argoproj.ArgoCD) string {
	return fmt.Sprintf("%s-%s", cr.Name, suffix)
}

// getLogLevel returns the log level for a specified component if it is set or returns the default log level if it is not set
func getLogLevel(logField string) string {

	switch strings.ToLower(logField) {
	case "debug",
		"info",
		"warn",
		"error":
		return logField
	}
	return common.ArgoCDDefaultLogLevel
}

// getLogFormat returns the log format for a specified component if it is set or returns the default log format if it is not set
func getLogFormat(logField string) string {
	switch strings.ToLower(logField) {
	case "text",
		"json":
		return logField
	}
	return common.ArgoCDDefaultLogFormat
}

// boolPtr returns a pointer to val
func boolPtr(val bool) *bool {
	return &val
}

// contains returns true if a string is part of the given slice.
func contains(s []string, g string) bool {
	for _, a := range s {
		if a == g {
			return true
		}
	}
	return false
}

// appendUniqueArgs appends extraArgs to cmd while ignoring any duplicate flags.
func appendUniqueArgs(cmd []string, extraArgs []string) []string {
	existing := map[string]string{}
	repeated := map[string]map[string]bool{}
	nonRepeatableFlags := map[string]bool{}
	result := []string{}

	// Helper to add flag+val to result
	add := func(flag, val string) {
		result = append(result, flag)
		if val != "" {
			result = append(result, val)
		}
	}

	// Process original cmd and treat its flags as non-repeatable
	for i := 0; i < len(cmd); i++ {
		arg := cmd[i]
		if strings.HasPrefix(arg, "--") {
			val := ""
			if i+1 < len(cmd) && !strings.HasPrefix(cmd[i+1], "--") {
				val = cmd[i+1]
				i++
			}
			if repeated[arg] == nil {
				repeated[arg] = map[string]bool{}
			}
			repeated[arg][val] = true
			existing[arg] = val
			nonRepeatableFlags[arg] = true // flags from cmd are non-repeatable
			add(arg, val)
		} else {
			result = append(result, arg)
		}
	}

	// Process extraArgs
	for i := 0; i < len(extraArgs); i++ {
		arg := extraArgs[i]
		if strings.HasPrefix(arg, "--") {
			val := ""
			if i+1 < len(extraArgs) && !strings.HasPrefix(extraArgs[i+1], "--") {
				val = extraArgs[i+1]
				i++
			}

			// Skip if this flag+val combo already exists
			if repeated[arg] != nil && repeated[arg][val] {
				continue
			}

			if nonRepeatableFlags[arg] {
				// Remove the existing non-repeatable flag (and its value)
				newResult := []string{}
				skipNext := false
				for j := 0; j < len(result); j++ {
					if skipNext {
						skipNext = false
						continue
					}
					if result[j] == arg {
						if j+1 < len(result) && !strings.HasPrefix(result[j+1], "--") {
							skipNext = true
						}
						continue
					}
					newResult = append(newResult, result[j])
				}
				result = newResult

				// Replace with new value
				repeated[arg] = map[string]bool{val: true}
				existing[arg] = val
				add(arg, val)
			} else {
				// Allow repeated if not seen before
				if repeated[arg] == nil {
					repeated[arg] = map[string]bool{}
				}
				repeated[arg][val] = true
				add(arg, val)
			}
		} else {
			result = append(result, arg)
		}
	}

	return result
}

// addKubernetesData checks for any Kubernetes-specific labels or annotations
// in the live object and updates the source object to ensure critical metadata
// (like scheduling, topology, or lifecycle information) is retained.
// This helps avoid loss of important Kubernetes-managed metadata during updates.
func addKubernetesData(source map[string]string, live map[string]string) {

	// List of Kubernetes-specific substrings (wildcard match)
	patterns := []string{
		"*kubernetes.io*",
		"*k8s.io*",
		"*openshift.io*",
	}

	for key, value := range live {
		found := glob.MatchStringInList(patterns, key, glob.GLOB)
		if found {
			// Don't override values already present in the source object.
			// This allows the operator to update Kubernetes specific data when needed.
			if _, ok := source[key]; !ok {
				source[key] = value
			}
		}
	}
}

func proxyEnvVars(vars ...corev1.EnvVar) []corev1.EnvVar {
	result := []corev1.EnvVar{}
	result = append(result, vars...)
	proxyKeys := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	for _, p := range proxyKeys {
		if k, v := caseInsensitiveGetenv(p); k != "" {
			result = append(result, corev1.EnvVar{Name: k, Value: v})
		}
	}
	return result
}
func caseInsensitiveGetenv(s string) (string, string) {
	if v := os.Getenv(s); v != "" {
		return s, v
	}
	ls := strings.ToLower(s)
	if v := os.Getenv(ls); v != "" {
		return ls, v
	}
	return "", ""
}
func allowedNamespace(current string, namespaces string) bool {

	clusterConfigNamespaces := splitList(namespaces)
	if len(clusterConfigNamespaces) > 0 {
		if clusterConfigNamespaces[0] == "*" {
			return true
		}

		for _, n := range clusterConfigNamespaces {
			if n == current {
				return true
			}
		}
	}
	return false
}
func splitList(s string) []string {
	elems := strings.Split(s, ",")
	for i := range elems {
		elems[i] = strings.TrimSpace(elems[i])
	}
	return elems
}

// newClusterRoleBindingWithname creates a new ClusterRoleBinding with the given name for the given ArgCD.
func newClusterRoleBindingWithname(name string, cr *argoproj.ArgoCD) *v1.ClusterRoleBinding {
	roleBinding := newClusterRoleBinding(cr)
	roleBinding.Name = GenerateUniqueResourceName(name, cr)

	labels := roleBinding.ObjectMeta.Labels
	labels[common.ArgoCDKeyName] = name
	roleBinding.ObjectMeta.Labels = labels

	return roleBinding
}

// newClusterRoleBinding returns a new ClusterRoleBinding instance.
func newClusterRoleBinding(cr *argoproj.ArgoCD) *v1.ClusterRoleBinding {
	return &v1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Labels:      argoutil.LabelsForCluster(cr),
			Annotations: argoutil.AnnotationsForCluster(cr),
		},
	}
}

func policyRuleForApplicationSetController() []v1.PolicyRule {
	return []v1.PolicyRule{
		// ApplicationSet
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{
				"applications",
				"applicationsets",
				"applicationsets/finalizers",
			},
			Verbs: []string{
				"create",
				"delete",
				"get",
				"list",
				"patch",
				"update",
				"watch",
			},
		},
		// ApplicationSet Status
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{
				"applicationsets/status",
			},
			Verbs: []string{
				"get",
				"patch",
				"update",
			},
		},
		// AppProjects
		{
			APIGroups: []string{"argoproj.io"},
			Resources: []string{
				"appprojects",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},

		// Events
		{
			APIGroups: []string{""},
			Resources: []string{
				"events",
			},
			Verbs: []string{
				"create",
				"get",
				"list",
				"patch",
				"watch",
			},
		},

		// ConfigMaps
		{
			APIGroups: []string{""},
			Resources: []string{
				"configmaps",
			},
			Verbs: []string{
				"create",
				"update",
				"delete",
				"get",
				"list",
				"patch",
				"watch",
			},
		},

		// Secrets
		{
			APIGroups: []string{""},
			Resources: []string{
				"secrets",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},

		// Deployments
		{
			APIGroups: []string{"apps", "extensions"},
			Resources: []string{
				"deployments",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
			},
		},

		// leases
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{
				"leases",
			},
			Verbs: []string{
				"create",
				"delete",
				"get",
				"list",
				"patch",
				"update",
				"watch",
			},
		},
	}
}

// getCAConfigMapName will return the CA ConfigMap name for the given ArgoCD.
func getCAConfigMapName(cr *argoproj.ArgoCD) string {
	if len(cr.Spec.TLS.CA.ConfigMapName) > 0 {
		return cr.Spec.TLS.CA.ConfigMapName
	}
	return nameWithSuffix(common.ArgoCDCASuffix, cr)
}

// getSCMRootCAConfigMapName will return the SCMRootCA ConfigMap name for the given ArgoCD ApplicationSet Controller.
func getSCMRootCAConfigMapName(cr *argoproj.ArgoCD) string {
	if cr.Spec.ApplicationSet.SCMRootCAConfigMap != "" && len(cr.Spec.ApplicationSet.SCMRootCAConfigMap) > 0 {
		return cr.Spec.ApplicationSet.SCMRootCAConfigMap
	}
	return ""
}

// newConfigMapWithName creates a new ConfigMap with the given name for the given ArgCD.
func newConfigMapWithName(name string, cr *argoproj.ArgoCD) *corev1.ConfigMap {
	cm := newConfigMap(cr)
	cm.ObjectMeta.Name = name

	lbls := cm.ObjectMeta.Labels
	lbls[common.ArgoCDKeyName] = name
	cm.ObjectMeta.Labels = lbls

	return cm
}

// newConfigMap returns a new ConfigMap instance for the given ArgoCD.
func newConfigMap(cr *argoproj.ArgoCD) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    argoutil.LabelsForCluster(cr),
		},
	}
}

// newRoleBindingWithname creates a new RoleBinding with the given name for the given ArgCD.
func newRoleBindingWithname(name string, cr *argoproj.ArgoCD) *v1.RoleBinding {
	roleBinding := newRoleBinding(cr)
	roleBinding.ObjectMeta.Name = fmt.Sprintf("%s-%s", cr.Name, name)

	labels := roleBinding.ObjectMeta.Labels
	labels[common.ArgoCDKeyName] = name
	roleBinding.ObjectMeta.Labels = labels

	return roleBinding
}

// newRoleBinding returns a new RoleBinding instance.
func newRoleBinding(cr *argoproj.ArgoCD) *v1.RoleBinding {
	return &v1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Labels:      argoutil.LabelsForCluster(cr),
			Annotations: argoutil.AnnotationsForCluster(cr),
			Namespace:   cr.Namespace,
		},
	}
}

// newRole returns a new Role instance.
func newRole(name string, rules []v1.PolicyRule, cr *argoproj.ArgoCD) *v1.Role {
	return &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateResourceName(name, cr),
			Namespace: cr.Namespace,
			Labels:    argoutil.LabelsForCluster(cr),
		},
		Rules: rules,
	}
}

// newServiceWithSuffix returns a new Service instance for the given ArgoCD using the given suffix.
func newServiceWithSuffix(suffix string, component string, cr *argoproj.ArgoCD) *corev1.Service {
	return newServiceWithName(fmt.Sprintf("%s-%s", cr.Name, suffix), component, cr)
}

// newServiceWithName returns a new Service instance for the given ArgoCD using the given name.
func newServiceWithName(name string, component string, cr *argoproj.ArgoCD) *corev1.Service {
	svc := newService(cr)
	svc.ObjectMeta.Name = name

	lbls := svc.ObjectMeta.Labels
	lbls[common.ArgoCDKeyName] = name
	lbls[common.ArgoCDKeyComponent] = component
	svc.ObjectMeta.Labels = lbls

	return svc
}

// newService returns a new Service for the given ArgoCD instance.
func newService(cr *argoproj.ArgoCD) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    argoutil.LabelsForCluster(cr),
		},
	}
}

func generateResourceName(argoComponentName string, cr *argoproj.ArgoCD) string {
	return cr.Name + "-" + argoComponentName
}
