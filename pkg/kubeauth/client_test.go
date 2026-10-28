package kubeauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	configapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNewClientNonexistentKubeconfig(t *testing.T) {
	if _, err := NewClient(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing explicit config should fail")
	}
}

func TestKubeconfigEnvironmentAndContextOverride(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(dir, name)
		cfg := configapi.Config{Clusters: map[string]*configapi.Cluster{name: {Server: "https://" + name + ".invalid"}}, AuthInfos: map[string]*configapi.AuthInfo{name: {}}, Contexts: map[string]*configapi.Context{name: {Cluster: name, AuthInfo: name}}, CurrentContext: name}
		if err := clientcmd.WriteToFile(cfg, path); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	t.Setenv("KUBECONFIG", paths[0]+string(os.PathListSeparator)+paths[1])
	client, err := NewClient("", "second")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.clientset.CoreV1().RESTClient().Get().URL().Host; got != "second.invalid" {
		t.Fatalf("selected host=%s", got)
	}
	client, err = NewClient(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := client.clientset.CoreV1().RESTClient().Get().URL().Host; got != "first.invalid" {
		t.Fatalf("explicit config host=%s", got)
	}
}

func TestPaginatedListsIncludeAllPodPhases(t *testing.T) {
	var pvcPages, podPages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("limit") != "500" || r.URL.Query().Get("fieldSelector") != "" {
			t.Errorf("unexpected list query %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/api/v1/persistentvolumeclaims":
			pvcPages.Add(1)
			page := corev1.PersistentVolumeClaimList{TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaimList", APIVersion: "v1"}}
			if r.URL.Query().Get("continue") == "" {
				page.Continue = "second"
				page.Items = []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "first"}}}
			} else {
				page.Items = []corev1.PersistentVolumeClaim{{ObjectMeta: metav1.ObjectMeta{Name: "second"}}}
			}
			if err := json.NewEncoder(w).Encode(page); err != nil {
				t.Error(err)
			}
		case "/api/v1/pods":
			podPages.Add(1)
			page := corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}}
			if r.URL.Query().Get("continue") == "" {
				page.Continue = "second"
				page.Items = []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodPending}}}
			} else {
				page.Items = []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodUnknown}}}
			}
			if err := json.NewEncoder(w).Encode(page); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{clientset: clientset}
	pvcs, err := client.ListPVCs(context.Background())
	if err != nil || len(pvcs) != 2 || pvcPages.Load() != 2 {
		t.Fatalf("PVC pages=%d result=%v err=%v", pvcPages.Load(), pvcs, err)
	}
	pods, err := client.ListPods(context.Background())
	if err != nil || len(pods) != 2 || podPages.Load() != 2 || pods[0].Status.Phase != corev1.PodPending || pods[1].Status.Phase != corev1.PodUnknown {
		t.Fatalf("Pod pages=%d result=%v err=%v", podPages.Load(), pods, err)
	}
}

func TestExecCredentialFixture(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("exec plugin credentials were not used")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","metadata":{},"items":[]}`))
	}))
	defer server.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config")
	cfg := configapi.Config{
		Clusters: map[string]*configapi.Cluster{"fixture": {Server: server.URL, InsecureSkipTLSVerify: true}},
		AuthInfos: map[string]*configapi.AuthInfo{"fixture": {Exec: &configapi.ExecConfig{
			Command: executable, Args: []string{"-test.run=TestExecCredentialHelperProcess"},
			APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: configapi.NeverExecInteractiveMode,
			Env: []configapi.ExecEnvVar{{Name: "ORPHANSCAN_TEST_EXEC_HELPER", Value: "1"}},
		}}},
		Contexts:       map[string]*configapi.Context{"fixture": {Cluster: "fixture", AuthInfo: "fixture"}},
		CurrentContext: "fixture",
	}
	if err := clientcmd.WriteToFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListPods(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExecCredentialHelperProcess(t *testing.T) {
	if os.Getenv("ORPHANSCAN_TEST_EXEC_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString(`{"kind":"ExecCredential","apiVersion":"client.authentication.k8s.io/v1","status":{"token":"fixture-token"}}`)
	os.Exit(0)
}
