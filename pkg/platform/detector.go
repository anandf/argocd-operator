package platform

// DEPRECATED: Runtime platform detection is deprecated in favor of build-time platform selection.
// Use NewPlatform() which is implemented differently based on build tags.
//
// The platform is now determined at build time using Go build tags:
//   - Build without tags (default): Kubernetes platform
//   - Build with -tags openshift: OpenShift platform
//
// This file is kept for backwards compatibility but will be removed in a future release.
//
// For runtime API detection (e.g., Route vs Ingress), use the APIDetector instead.
