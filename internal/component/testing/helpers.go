package testing

import (
	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	TestNamespace  = "argocd"
	TestArgoCDName = "argocd"
)

// NewTestScheme returns a runtime.Scheme with the standard Kubernetes types
// and the ArgoCD CRD types registered.
func NewTestScheme() *runtime.Scheme {
	s := scheme.Scheme
	_ = argoproj.AddToScheme(s)
	return s
}

// NewTestClient creates a fake controller-runtime client with the given objects.
func NewTestClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

// NewTestArgoCD creates a basic ArgoCD CR for testing.
func NewTestArgoCD(opts ...func(*argoproj.ArgoCD)) *argoproj.ArgoCD {
	cr := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestArgoCDName,
			Namespace: TestNamespace,
		},
	}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

// WithHA sets the HA mode on the ArgoCD CR.
func WithHA(enabled bool) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.HA.Enabled = enabled
	}
}

// WithSSO configures SSO with dex provider on the ArgoCD CR.
func WithSSO(provider argoproj.SSOProviderType) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.SSO = &argoproj.ArgoCDSSOSpec{
			Provider: provider,
		}
	}
}

// WithExternalRedis sets an external Redis URL on the ArgoCD CR.
func WithExternalRedis(url string) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.Redis.Remote = &url
	}
}

// WithServerDisabled disables the server component.
func WithServerDisabled() func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		disabled := false
		a.Spec.Server.Enabled = &disabled
	}
}

// WithRepoDisabled disables the repo server component.
func WithRepoDisabled() func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		disabled := false
		a.Spec.Repo.Enabled = &disabled
	}
}

// WithRepoRemote sets a remote repo server URL.
func WithRepoRemote(url string) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.Repo.Remote = &url
	}
}

// WithServerIngress enables ingress on the server.
func WithServerIngress(enabled bool) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.Server.Ingress.Enabled = enabled
	}
}

// WithServerRoute enables route on the server.
func WithServerRoute(enabled bool) func(*argoproj.ArgoCD) {
	return func(a *argoproj.ArgoCD) {
		a.Spec.Server.Route.Enabled = enabled
	}
}
