package cli

import (
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFormatError_NotFound(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Resource: "experiments"}, "my-exp")
	result := FormatError(err, "experiment", "my-exp", "default")
	if result == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.Error(), "not found in namespace") {
		t.Errorf("expected not-found hint, got: %s", result)
	}
	if !strings.Contains(result.Error(), "chaosctl experiment list") {
		t.Errorf("expected list hint, got: %s", result)
	}
}

func TestFormatError_Forbidden(t *testing.T) {
	err := apierrors.NewForbidden(schema.GroupResource{Resource: "experiments"}, "my-exp", errors.New("denied"))
	result := FormatError(err, "experiment", "my-exp", "default")
	if !strings.Contains(result.Error(), "permission denied") {
		t.Errorf("expected permission hint, got: %s", result)
	}
}

func TestFormatError_ConnectionRefused(t *testing.T) {
	err := errors.New("dial tcp 127.0.0.1:6443: connection refused")
	result := FormatError(err, "experiment", "", "")
	if !strings.Contains(result.Error(), "cannot connect to cluster") {
		t.Errorf("expected connection hint, got: %s", result)
	}
}

func TestFormatError_Nil(t *testing.T) {
	if FormatError(nil, "", "", "") != nil {
		t.Error("expected nil for nil input")
	}
}

func TestFormatError_Generic(t *testing.T) {
	err := errors.New("something unexpected")
	result := FormatError(err, "experiment", "x", "default")
	if result.Error() != "something unexpected" {
		t.Errorf("expected passthrough, got: %s", result)
	}
}
