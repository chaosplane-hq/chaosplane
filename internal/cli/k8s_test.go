package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveKubeconfig_Flag(t *testing.T) {
	got := ResolveKubeconfig("/custom/path")
	if got != "/custom/path" {
		t.Errorf("expected /custom/path, got %s", got)
	}
}

func TestResolveKubeconfig_Env(t *testing.T) {
	t.Setenv("KUBECONFIG", "/env/path")
	got := ResolveKubeconfig("")
	if got != "/env/path" {
		t.Errorf("expected /env/path, got %s", got)
	}
}

func TestResolveKubeconfig_Default(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	got := ResolveKubeconfig("")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".kube", "config")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}
