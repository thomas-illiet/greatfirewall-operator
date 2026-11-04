package operator

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greatfirewall/operator/internal/config"
	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/policy"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestStartErrors rejects bad API settings before attempting Kubernetes initialization.
func TestStartErrors(t *testing.T) {
	if Start(context.Background(), config.Operator{}) == nil {
		t.Fatal("invalid API configuration accepted")
	}
}

// TestManagerIntegration exercises real API watches, startup, leader election and shutdown.
func TestManagerIntegration(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run the real Kubernetes API integration test")
	}
	environment := &envtest.Environment{}
	restConfig, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})

	directory := t.TempDir()
	kubeconfigPath := filepath.Join(directory, "kubeconfig")
	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"local": {Server: restConfig.Host, CertificateAuthorityData: restConfig.CAData},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"local": {ClientCertificateData: restConfig.CertData, ClientKeyData: restConfig.KeyData},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"local": {Cluster: "local", AuthInfo: "local"},
		},
		CurrentContext: "local",
	}
	if err := clientcmd.WriteToFile(kubeconfig, kubeconfigPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfigPath)
	runManagerScenario(t, directory)
}

// runManagerScenario creates a labelled pod and verifies reconciliation through the real API.
func runManagerScenario(t *testing.T, directory string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profile":"dev","revision":"1","egress":[]}`))
	}))
	defer server.Close()

	settings := integrationSettings(t, directory, server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Start(ctx, settings) }()

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{},
	)
	restConfig, err := loader.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	kube, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	createIntegrationWorkload(t, ctx, kube)
	waitForPolicy(t, ctx, kube, done)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// integrationSettings writes ephemeral TLS trust and token files for the real manager.
func integrationSettings(t *testing.T, directory string, server *httptest.Server) config.Operator {
	t.Helper()
	tokenFile := filepath.Join(directory, "token")
	caFile := filepath.Join(directory, "ca.pem")
	files := map[string][]byte{
		tokenFile: []byte("0123456789abcdef0123456789abcdef"),
		caFile:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}),
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return config.Operator{
		APIURL: server.URL, TokenFile: tokenFile, CAFile: caFile,
		Namespace: "gfw-system", RefreshInterval: 100 * time.Millisecond,
		HTTPTimeout: time.Second, HealthAddress: "0", MetricsAddress: "0",
		LeaderElection: true, IPv6Disabled: true,
		DNS: policy.DNS{Addresses: []string{"10.96.0.10"}},
	}
}

// createIntegrationWorkload creates only disposable resources inside the envtest API server.
func createIntegrationWorkload(t *testing.T, ctx context.Context, kube client.Client) {
	t.Helper()
	for _, name := range []string{"gfw-system", "demo"} {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := kube.Create(ctx, ns); err != nil {
			t.Fatal(err)
		}
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sandbox", Namespace: "demo",
			Labels: map[string]string{identity.ProfileLabel: "dev"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "shell", Image: "busybox:1.37"}},
		},
	}
	if err := kube.Create(ctx, pod); err != nil {
		t.Fatal(err)
	}
}

// waitForPolicy waits for informer-driven reconciliation, with a bounded failure timeout.
func waitForPolicy(t *testing.T, ctx context.Context, kube client.Client, done <-chan error) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	key := client.ObjectKey{Namespace: "demo", Name: identity.ObjectName("dev")}
	for {
		select {
		case err := <-done:
			t.Fatalf("manager stopped before reconciliation: %v", err)
		case <-timeout.C:
			t.Fatal("profile policy was not reconciled")
		case <-ticker.C:
			compiled := new(networkingv1.NetworkPolicy)
			if kube.Get(ctx, key, compiled) == nil && len(compiled.Spec.Egress) == 1 {
				return
			}
		}
	}
}
