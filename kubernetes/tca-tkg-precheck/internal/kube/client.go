package kube

import (
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Options struct {
	KubeContext string
}

type Client struct {
	RESTConfig *rest.Config

	Clientset  *kubernetes.Clientset
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
	RawConfig  clientcmdapiConfig
	Context    string
	Cluster    string
	AuthInfo   string
	Namespace  string
	HasRawInfo bool
}

// clientcmdapiConfig is a small subset of clientcmdapi.Config so we can avoid
// exporting the full type in our public API while still surfacing useful metadata.
type clientcmdapiConfig struct {
	CurrentContext string
}

func NewClient(opts Options) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if opts.KubeContext != "" {
		overrides.CurrentContext = opts.KubeContext
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restCfg, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}

	// Try to capture the raw kubeconfig metadata (best-effort).
	raw, rawErr := cfg.RawConfig()
	c := &Client{
		RESTConfig: restCfg,
	}
	if rawErr == nil {
		ctxName := raw.CurrentContext
		c.RawConfig = clientcmdapiConfig{CurrentContext: ctxName}
		c.Context = ctxName
		if ctx, ok := raw.Contexts[ctxName]; ok && ctx != nil {
			c.Cluster = ctx.Cluster
			c.AuthInfo = ctx.AuthInfo
			c.Namespace = ctx.Namespace
		}
		c.HasRawInfo = true
	}

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("discovery client: %w", err)
	}

	c.Clientset = cs
	c.Dynamic = dc
	c.Discovery = disc
	return c, nil
}
