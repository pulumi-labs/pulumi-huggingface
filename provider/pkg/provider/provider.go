// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package provider implements the Hugging Face Pulumi provider.
//
// Resources:
//   - InferenceEndpoint — dedicated compute for production model serving
//   - Space             — interactive ML demos (Gradio, Docker, static)
//   - Repository        — model and dataset repository containers
//   - Webhook, Collection, CollectionItem, RepositoryBranch, RepositoryTag, Bucket
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
	Token *string `pulumi:"token,optional" provider:"secret"`
}

// Annotate describes the provider config fields and wires their environment
// variable fallbacks; infer reads env defaults only from SetDefault, not from
// struct tags.
func (c *ProviderConfig) Annotate(a infer.Annotator) {
	a.SetDefault(&c.Token, nil, "HUGGINGFACE_TOKEN")
	a.Describe(&c.Token, "Hugging Face API token. Overridable per resource. Falls back to the HUGGINGFACE_TOKEN environment variable.")
}

// New returns the configured Hugging Face Pulumi provider.
func newProvider() (pgp.Provider, error) {
	return infer.NewProviderBuilder().
		WithDisplayName("Hugging Face").
		WithDescription(
			"Experimental: manage Hugging Face resources — Inference Endpoints, Spaces, and Hub repositories — "+
				"as infrastructure with Pulumi.",
		).
		WithHomepage("https://huggingface.co").
		WithRepository("https://github.com/pulumi-labs/pulumi-huggingface").
		WithPublisher("Pulumi Labs").
		WithLanguageMap(map[string]any{
			"go": map[string]any{"generateResourceContainerTypes": true, "respectSchemaVersion": true},
			"nodejs": map[string]any{
				"packageName":          "@pulumi-labs/huggingface",
				"packageDescription":   "Experimental: manage Hugging Face resources — Inference Endpoints, Spaces, and Hub repositories — as infrastructure with Pulumi.",
				"respectSchemaVersion": true,
			},
			"python": map[string]any{
				"packageName":          "pulumi_labs_huggingface",
				"respectSchemaVersion": true,
				"pyproject":            map[string]any{"enabled": true},
			},
			"csharp": map[string]any{"rootNamespace": "Pulumi.Labs", "respectSchemaVersion": true},
			"java":   map[string]any{"basePackage": "com.pulumi.labs", "respectSchemaVersion": true},
		}).
		WithGoImportPath("github.com/pulumi-labs/pulumi-huggingface/sdk/go/huggingface").
		WithPluginDownloadURL("github://api.github.com/pulumi-labs/pulumi-huggingface").
		WithLicense("Apache-2.0").
		WithKeywords("pulumi", "huggingface", "ai", "machine-learning", "category/cloud").
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{
			"provider": "index",
		}).
		WithConfig(infer.Config(ProviderConfig{})).
		WithResources(
			infer.Resource[*InferenceEndpoint, InferenceEndpointArgs, InferenceEndpointState](&InferenceEndpoint{}),
			infer.Resource[*Space, SpaceArgs, SpaceState](&Space{}),
			infer.Resource[*Repository, RepositoryArgs, RepositoryState](&Repository{}),
			infer.Resource[*Webhook, WebhookArgs, WebhookState](&Webhook{}),
			infer.Resource[*Collection, CollectionArgs, CollectionState](&Collection{}),
			infer.Resource[*CollectionItem, CollectionItemArgs, CollectionItemState](&CollectionItem{}),
			infer.Resource[*RepositoryBranch, RepositoryBranchArgs, RepositoryBranchState](&RepositoryBranch{}),
			infer.Resource[*RepositoryTag, RepositoryTagArgs, RepositoryTagState](&RepositoryTag{}),
			infer.Resource[*Bucket, BucketArgs, BucketState](&Bucket{}),
		).
		WithFunctions(
			infer.Function[*GetWhoami, GetWhoamiArgs, GetWhoamiResult](&GetWhoami{}),
			infer.Function[*GetInferenceEndpoint, GetInferenceEndpointArgs, InferenceEndpointSummary](&GetInferenceEndpoint{}),
			infer.Function[*ListInferenceEndpoints, ListInferenceEndpointsArgs, ListInferenceEndpointsResult](&ListInferenceEndpoints{}),
			infer.Function[*GetSpace, GetSpaceArgs, GetSpaceResult](&GetSpace{}),
			infer.Function[*GetRepository, GetRepositoryArgs, GetRepositoryResult](&GetRepository{}),
			infer.Function[*GetWebhook, GetWebhookArgs, GetWebhookResult](&GetWebhook{}),
			infer.Function[*GetCollection, GetCollectionArgs, GetCollectionResult](&GetCollection{}),
			infer.Function[*GetCollectionItem, GetCollectionItemArgs, GetCollectionItemResult](&GetCollectionItem{}),
			infer.Function[*GetRepositoryBranch, GetRepositoryBranchArgs, GetRepositoryBranchResult](&GetRepositoryBranch{}),
			infer.Function[*GetRepositoryTag, GetRepositoryTagArgs, GetRepositoryTagResult](&GetRepositoryTag{}),
			infer.Function[*GetBucket, GetBucketArgs, GetBucketResult](&GetBucket{}),
		).
		Build()
}
