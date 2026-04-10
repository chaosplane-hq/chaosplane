package cli

import (
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func FormatError(err error, resourceKind, name, namespace string) error {
	if err == nil {
		return nil
	}

	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%s '%s' not found in namespace '%s'. Try: chaosctl %s list -n %s",
			resourceKind, name, namespace, strings.ToLower(resourceKind), namespace)
	}
	if apierrors.IsForbidden(err) {
		return fmt.Errorf("permission denied. Check your kubeconfig and RBAC settings")
	}
	if apierrors.IsConflict(err) {
		return fmt.Errorf("%s '%s' was modified by another process. Please retry", resourceKind, name)
	}

	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return fmt.Errorf("cannot connect to cluster. Is your kubeconfig correct? Try: kubectl cluster-info")
	}
	if strings.Contains(msg, "certificate") {
		return fmt.Errorf("TLS certificate error. Check your kubeconfig certificates or try: kubectl cluster-info")
	}

	return err
}
