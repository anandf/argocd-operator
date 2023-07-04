package v1alpha1

import (
	"fmt"

	"github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	"github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this CronJob to the Hub version (v1).
func (src *v1alpha1.ArgoCD) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.ArgoCD)
	fmt.Println(dst.APIVersion)
	return nil
}
