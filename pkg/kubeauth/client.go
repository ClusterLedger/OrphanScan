package kubeauth

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct{ clientset kubernetes.Interface }

// NewClient honors an explicit path, otherwise KUBECONFIG and the default
// kubeconfig loading rules. An optional context overrides the current context.
func NewClient(kubeconfigPath string, contextName ...string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = kubeconfigPath
	overrides := &clientcmd.ConfigOverrides{}
	if len(contextName) > 0 {
		overrides.CurrentContext = contextName[0]
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	// Server warning headers can contain arbitrary identifiers. The report's
	// structured diagnostics provide the public-facing status instead.
	config.WarningHandler = rest.NoWarnings{}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{clientset: clientset}, nil
}

func (c *Client) ListPVCs(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
	var pvcs []corev1.PersistentVolumeClaim
	options := metav1.ListOptions{Limit: 500}
	for {
		page, err := c.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, options)
		if err != nil {
			return nil, err
		}
		pvcs = append(pvcs, page.Items...)
		if page.Continue == "" {
			return pvcs, nil
		}
		options.Continue = page.Continue
	}
}

// ListPods includes every phase: Pending, Unknown, and completed Pod references
// also protect a PVC from an orphan candidate finding.
func (c *Client) ListPods(ctx context.Context) ([]corev1.Pod, error) {
	var pods []corev1.Pod
	options := metav1.ListOptions{Limit: 500}
	for {
		page, err := c.clientset.CoreV1().Pods("").List(ctx, options)
		if err != nil {
			return nil, err
		}
		pods = append(pods, page.Items...)
		if page.Continue == "" {
			return pods, nil
		}
		options.Continue = page.Continue
	}
}
