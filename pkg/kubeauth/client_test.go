package kubeauth

import (
	"testing"
)

func TestNewClient_NonexistentKubeconfig(t *testing.T) {
	_, err := NewClient("/nonexistent/path/kubeconfig")
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}
}
