package component

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
	"github.com/argoproj-labs/argocd-operator/internal/template"
	"github.com/argoproj/argo-cd/v3/util/glob"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetResourceNameForApplicationSetSourceNamespaces returns the name of the role/rolebinding
// for the source namespaces for applicationset-controller in the format of "argocdName-argocdNamespace-applicationset"
func GetResourceNameForApplicationSetSourceNamespaces(cr *argoproj.ArgoCD) string {
	return fmt.Sprintf("%s-%s-applicationset", cr.Name, cr.Namespace)
}

// IdentifyDeploymentDifference is a simple comparison of the contents of two
// deployments, returning "" if they are the same, otherwise returning the name
// of the field that changed.
func IdentifyDeploymentDifference(x appsv1.Deployment, y appsv1.Deployment) string {

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

// GetApplicationSetContainerImage returns the container image for the ApplicationSet controller.
func GetApplicationSetContainerImage(cr *argoproj.ArgoCD) string {

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

// GetApplicationSetResources will return the ResourceRequirements for the Application Sets container.
func GetApplicationSetResources(cr *argoproj.ArgoCD) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}

	// Allow override of resource requirements from CR
	if cr.Spec.ApplicationSet.Resources != nil {
		resources = *cr.Spec.ApplicationSet.Resources
	}

	return resources
}

// SetAppSetLabels sets the standard labels for ApplicationSet controller resources.
func SetAppSetLabels(obj *metav1.ObjectMeta) {
	obj.Labels["app.kubernetes.io/name"] = "argocd-applicationset-controller"
	obj.Labels["app.kubernetes.io/part-of"] = "argocd"
	obj.Labels["app.kubernetes.io/component"] = "controller"
}

// NewServiceAccountWithName creates a new ServiceAccount with the given name for the given ArgoCD.
func NewServiceAccountWithName(name string, cr *argoproj.ArgoCD) *corev1.ServiceAccount {
	sa := newServiceAccount(cr)
	sa.ObjectMeta.Name = GetServiceAccountName(cr.Name, name)

	lbls := sa.ObjectMeta.Labels
	lbls[common.ArgoCDKeyName] = name
	sa.ObjectMeta.Labels = lbls

	return sa
}

// GetServiceAccountName returns the service account name for the given ArgoCD and component name.
func GetServiceAccountName(crName, name string) string {
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

// GetRepoServerAddress will return the Argo CD repo server address.
func GetRepoServerAddress(cr *argoproj.ArgoCD) string {
	if cr.Spec.Repo.IsRemote() {
		return *cr.Spec.Repo.Remote
	}
	return fqdnServiceRef("repo-server", common.ArgoCDDefaultRepoServerPort, cr)
}

// fqdnServiceRef will return the FQDN referencing a specific service name, as set up by the operator, with the
// given port.
func fqdnServiceRef(service string, port int, cr *argoproj.ArgoCD) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", fmt.Sprintf("%s-repo-server", cr.Name), cr.Namespace, port)
}


// GetLogLevel returns the log level for a specified component if it is set or returns the default log level if it is not set.
func GetLogLevel(logField string) string {

	switch strings.ToLower(logField) {
	case "debug",
		"info",
		"warn",
		"error":
		return logField
	}
	return common.ArgoCDDefaultLogLevel
}

// GetLogFormat returns the log format for a specified component if it is set or returns the default log format if it is not set.
func GetLogFormat(logField string) string {
	switch strings.ToLower(logField) {
	case "text",
		"json":
		return logField
	}
	return common.ArgoCDDefaultLogFormat
}

// GetArgoContainerImage returns the main ArgoCD container image.
// This is used by components that need the core ArgoCD image (e.g., for init containers).
func GetArgoContainerImage(cr *argoproj.ArgoCD) string {
	img := cr.Spec.Image
	if img == "" {
		img = common.ArgoCDDefaultArgoImage
	}
	tag := cr.Spec.Version
	if tag == "" {
		tag = common.ArgoCDDefaultArgoVersion
	}
	if e := os.Getenv(common.ArgoCDImageEnvName); e != "" && img == common.ArgoCDDefaultArgoImage && tag == common.ArgoCDDefaultArgoVersion {
		return e
	}
	return argoutil.CombineImageTag(img, tag)
}

// ApplyNodePlacement adds NodeSelector and Tolerations from the CR's NodePlacement
// spec to the template data. This is a shared helper to avoid duplicating
// the same nil-check pattern across all component controllers.
func ApplyNodePlacement(data *template.TemplateData, np *argoproj.ArgoCDNodePlacementSpec) *template.TemplateData {
	if np != nil {
		if np.NodeSelector != nil {
			data.WithExtra("NodeSelector", np.NodeSelector)
		}
		if np.Tolerations != nil {
			data.WithExtra("Tolerations", np.Tolerations)
		}
	}
	return data
}

// TemplateResources is a template-friendly representation of ResourceRequirements.
// resource.Quantity has a String() method on a pointer receiver, which means
// values in a map iteration within Go templates are not addressable and
// String() won't be called. This struct pre-converts quantities to strings.
type TemplateResources struct {
	Limits  map[string]string
	Requests map[string]string
}

// ResourcesToTemplate converts corev1.ResourceRequirements to a template-friendly
// format where all quantities are pre-converted to strings.
func ResourcesToTemplate(r corev1.ResourceRequirements) *TemplateResources {
	tr := &TemplateResources{}
	if r.Limits != nil {
		tr.Limits = make(map[string]string)
		for k, v := range r.Limits {
			tr.Limits[string(k)] = v.String()
		}
	}
	if r.Requests != nil {
		tr.Requests = make(map[string]string)
		for k, v := range r.Requests {
			tr.Requests[string(k)] = v.String()
		}
	}
	return tr
}

// BoolPtr returns a pointer to val.
func BoolPtr(val bool) *bool {
	return &val
}

// Contains returns true if a string is part of the given slice.
func Contains(s []string, g string) bool {
	for _, a := range s {
		if a == g {
			return true
		}
	}
	return false
}

// AppendUniqueArgs appends extraArgs to cmd while ignoring any duplicate flags.
func AppendUniqueArgs(cmd []string, extraArgs []string) []string {
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

// AddKubernetesData checks for any Kubernetes-specific labels or annotations
// in the live object and updates the source object to ensure critical metadata
// (like scheduling, topology, or lifecycle information) is retained.
func AddKubernetesData(source map[string]string, live map[string]string) {

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

// ProxyEnvVars returns environment variables for proxy configuration.
func ProxyEnvVars(vars ...corev1.EnvVar) []corev1.EnvVar {
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

// AllowedNamespace checks whether the given namespace is in the allowed list.
func AllowedNamespace(current string, namespaces string) bool {

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
