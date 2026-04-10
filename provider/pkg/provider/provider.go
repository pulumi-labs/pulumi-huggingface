// Package provider implements the Hugging Face Pulumi provider.
//
// Resources:
//   - InferenceEndpoint — dedicated compute for production model serving
//   - Space             — interactive ML demos (Gradio, Docker, static)
//   - Repository        — model and dataset repository containers
package provider

import (
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// ProviderConfig holds provider-level configuration shared across all resources.
type ProviderConfig struct {
	// Token is the Hugging Face API token. Can be overridden per resource.
	// If not set here, the HUGGINGFACE_TOKEN environment variable is consulted.
	Token *string `pulumi:"token,optional" provider:"secret" env:"HUGGINGFACE_TOKEN"`
}

func (c *ProviderConfig) Annotate(a infer.Annotator) {
	a.Describe(&c.Token, "Hugging Face API token. Overridable per resource. Falls back to the HUGGINGFACE_TOKEN environment variable.")
}

// New returns the configured Hugging Face Pulumi provider.
func New() (pgp.Provider, error) {
	return infer.NewProviderBuilder().
		WithDisplayName("huggingface").
		WithDescription(
			"Manage Hugging Face resources — Inference Endpoints, Spaces, and Hub repositories — "+
				"as infrastructure with Pulumi.",
		).
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{
			"provider": "index",
		}).
		WithConfig(infer.Config(ProviderConfig{})).
		WithResources(
			infer.Resource[*InferenceEndpoint, InferenceEndpointArgs, InferenceEndpointState](&InferenceEndpoint{}),
			infer.Resource[*Space, SpaceArgs, SpaceState](&Space{}),
			infer.Resource[*Repository, RepositoryArgs, RepositoryState](&Repository{}),
		).
		Build()
}
