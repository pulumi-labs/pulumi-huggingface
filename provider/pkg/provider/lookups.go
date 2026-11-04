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

package provider

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getWhoami
// ---------------------------------------------------------------------------

// GetWhoami returns the identity behind the API token.
type GetWhoami struct{}

// GetWhoamiArgs are the inputs of getWhoami.
type GetWhoamiArgs struct {
	Token *string `pulumi:"token,optional" provider:"secret"`
}

// GetWhoamiResult is the authenticated identity.
type GetWhoamiResult struct {
	Name     string   `pulumi:"name"`
	Type     string   `pulumi:"type"`
	FullName string   `pulumi:"fullName"`
	Orgs     []string `pulumi:"orgs"`
}

func (f *GetWhoami) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up the Hugging Face user (or service identity) behind the API token, and the organizations it belongs to.")
}

func (a *GetWhoamiArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
}

func (r *GetWhoamiResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.Name, "The user name (the default namespace for new resources).")
	ann.Describe(&r.Type, "The identity type, for example 'user'.")
	ann.Describe(&r.FullName, "The display name.")
	ann.Describe(&r.Orgs, "Names of the organizations the identity belongs to.")
}

func (f *GetWhoami) Invoke(ctx context.Context, req infer.FunctionRequest[GetWhoamiArgs]) (infer.FunctionResponse[GetWhoamiResult], error) {
	out := infer.FunctionResponse[GetWhoamiResult]{}
	client, _, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	w, err := client.Whoami()
	if err != nil {
		return out, fmt.Errorf("huggingface: look up the authenticated user: %w", err)
	}
	orgs := make([]string, 0, len(w.Orgs))
	for _, o := range w.Orgs {
		orgs = append(orgs, o.Name)
	}
	out.Output = GetWhoamiResult{Name: w.Name, Type: w.Type, FullName: w.FullName, Orgs: orgs}
	return out, nil
}

// ---------------------------------------------------------------------------
// getRepository
// ---------------------------------------------------------------------------

// GetRepository looks up a model or dataset repository.
type GetRepository struct{}

// GetRepositoryArgs selects the repository.
type GetRepositoryArgs struct {
	Token    *string `pulumi:"token,optional" provider:"secret"`
	RepoID   string  `pulumi:"repoId"`
	RepoType *string `pulumi:"repoType,optional"`
}

// GetRepositoryResult is the looked-up repository.
type GetRepositoryResult struct {
	RepoID   string `pulumi:"repoId"`
	RepoType string `pulumi:"repoType"`
	Private  bool   `pulumi:"private"`
	Gated    string `pulumi:"gated"`
	URL      string `pulumi:"url"`
}

func (f *GetRepository) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Hub model or dataset repository by ID.")
}

func (a *GetRepositoryArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.RepoID, "The repository, 'namespace/name'.")
	ann.Describe(&a.RepoType, "The repository type: 'model' (default) or 'dataset'.")
}

func (r *GetRepositoryResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.RepoID, "The repository, 'namespace/name'.")
	ann.Describe(&r.RepoType, "The repository type.")
	ann.Describe(&r.Private, "Whether the repository is private.")
	ann.Describe(&r.Gated, "Gated access: 'off', 'auto' or 'manual'.")
	ann.Describe(&r.URL, "The web URL of the repository.")
}

func (f *GetRepository) Invoke(ctx context.Context, req infer.FunctionRequest[GetRepositoryArgs]) (infer.FunctionResponse[GetRepositoryResult], error) {
	out := infer.FunctionResponse[GetRepositoryResult]{}
	in := req.Input
	rt := resolveRepoType(in.RepoType)
	if rt != "model" && rt != "dataset" {
		return out, fmt.Errorf("huggingface: getRepository repoType must be 'model' or 'dataset' (use getSpace for Spaces)")
	}
	ns, name, err := splitID(in.RepoID)
	if err != nil {
		return out, fmt.Errorf("huggingface: getRepository: %w", err)
	}
	client, _, err := clientFor(ctx, in.Token)
	if err != nil {
		return out, err
	}
	repo, err := client.GetRepo(rt, ns, name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up repository %q: %w", in.RepoID, err)
	}
	if repo == nil {
		return out, fmt.Errorf("huggingface: %s repository %q was not found", rt, in.RepoID)
	}
	id := ns + "/" + name
	out.Output = GetRepositoryResult{RepoID: id, RepoType: rt, Private: repo.Private, Gated: gatedValue(repo.Gated), URL: repoURL(rt, id)}
	return out, nil
}

// ---------------------------------------------------------------------------
// getSpace
// ---------------------------------------------------------------------------

// GetSpace looks up a Space.
type GetSpace struct{}

// GetSpaceArgs selects the Space.
type GetSpaceArgs struct {
	Token   *string `pulumi:"token,optional" provider:"secret"`
	SpaceID string  `pulumi:"spaceId"`
}

// GetSpaceResult is the looked-up Space. Secret values are write-only in the
// Hugging Face API, so only the secret names are returned.
type GetSpaceResult struct {
	SpaceID          string            `pulumi:"spaceId"`
	SDK              string            `pulumi:"sdk"`
	Private          bool              `pulumi:"private"`
	Hardware         string            `pulumi:"hardware"`
	SleepTimeSeconds *int              `pulumi:"sleepTimeSeconds,optional"`
	Stage            string            `pulumi:"stage"`
	URL              string            `pulumi:"url"`
	Variables        map[string]string `pulumi:"variables"`
	SecretKeys       []string          `pulumi:"secretKeys"`
}

func (f *GetSpace) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Space by ID. Secret values are not supported by the Hugging Face API: only the secret names are returned.")
}

func (a *GetSpaceArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.SpaceID, "The Space, 'namespace/name'.")
}

func (r *GetSpaceResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.SpaceID, "The Space, 'namespace/name'.")
	ann.Describe(&r.SDK, "The frontend framework.")
	ann.Describe(&r.Private, "Whether the Space is private.")
	ann.Describe(&r.Hardware, "The hardware flavor requested for the Space, or the current one when none is pending.")
	ann.Describe(&r.SleepTimeSeconds, "Idle seconds before the Space sleeps, when configured.")
	ann.Describe(&r.Stage, "The runtime stage, for example 'RUNNING' or 'SLEEPING'.")
	ann.Describe(&r.URL, "The web URL of the Space.")
	ann.Describe(&r.Variables, "The Space variables (non-secret), by name.")
	ann.Describe(&r.SecretKeys, "The names of the Space secrets. The values are not supported by the Hugging Face API.")
}

func (f *GetSpace) Invoke(ctx context.Context, req infer.FunctionRequest[GetSpaceArgs]) (infer.FunctionResponse[GetSpaceResult], error) {
	out := infer.FunctionResponse[GetSpaceResult]{}
	ns, name, err := splitID(req.Input.SpaceID)
	if err != nil {
		return out, fmt.Errorf("huggingface: getSpace: %w", err)
	}
	client, _, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	repo, err := client.GetRepo("space", ns, name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up space %q: %w", req.Input.SpaceID, err)
	}
	if repo == nil {
		return out, fmt.Errorf("huggingface: space %q was not found", req.Input.SpaceID)
	}
	vars, err := client.GetSpaceVariables(ns, name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up space %q: %w", req.Input.SpaceID, err)
	}
	secrets, err := client.GetSpaceSecretKeys(ns, name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up space %q: %w", req.Input.SpaceID, err)
	}
	res := GetSpaceResult{
		SpaceID: ns + "/" + name, Private: repo.Private, URL: repoURL("space", ns+"/"+name),
		Variables: map[string]string{}, SecretKeys: slices.Sorted(maps.Keys(secrets)),
	}
	if repo.SDK != nil {
		res.SDK = *repo.SDK
	}
	for k, v := range vars {
		res.Variables[k] = v.Value
	}
	if hw, ok := repo.EffectiveHardware(); ok {
		res.Hardware = hw
	}
	if repo.Runtime != nil {
		res.Stage = repo.Runtime.Stage
		res.SleepTimeSeconds = repo.Runtime.GCTimeout
	}
	out.Output = res
	return out, nil
}

// ---------------------------------------------------------------------------
// getInferenceEndpoint / listInferenceEndpoints
// ---------------------------------------------------------------------------

// InferenceEndpointSummary is the lookup view of an Inference Endpoint.
type InferenceEndpointSummary struct {
	Name               string   `pulumi:"name"`
	Namespace          string   `pulumi:"namespace"`
	EndpointType       string   `pulumi:"endpointType"`
	Status             string   `pulumi:"status"`
	StatusMessage      string   `pulumi:"statusMessage"`
	URL                string   `pulumi:"url"`
	ModelRepository    string   `pulumi:"modelRepository"`
	ModelFramework     string   `pulumi:"modelFramework"`
	ModelTask          string   `pulumi:"modelTask"`
	ModelRevision      *string  `pulumi:"modelRevision,optional"`
	Vendor             string   `pulumi:"vendor"`
	Region             string   `pulumi:"region"`
	Accelerator        string   `pulumi:"accelerator"`
	InstanceType       string   `pulumi:"instanceType"`
	InstanceSize       string   `pulumi:"instanceSize"`
	MinReplica         int      `pulumi:"minReplica"`
	MaxReplica         int      `pulumi:"maxReplica"`
	ScaleToZeroTimeout *int     `pulumi:"scaleToZeroTimeout,optional"`
	Tags               []string `pulumi:"tags"`
}

func (s *InferenceEndpointSummary) Annotate(a infer.Annotator) {
	a.Describe(&s.Name, "The endpoint name.")
	a.Describe(&s.Namespace, "The owning user or organization.")
	a.Describe(&s.EndpointType, "Access type: 'public', 'authenticated' or 'private'.")
	a.Describe(&s.Status, "Lifecycle state, for example 'running' or 'scaledToZero'.")
	a.Describe(&s.StatusMessage, "The status message reported by the API.")
	a.Describe(&s.URL, "The HTTPS URL for inference; empty until the endpoint is running.")
	a.Describe(&s.ModelRepository, "The model repository being served.")
	a.Describe(&s.ModelFramework, "The serving framework.")
	a.Describe(&s.ModelTask, "The inference task.")
	a.Describe(&s.ModelRevision, "The pinned model revision, when set.")
	a.Describe(&s.Vendor, "The cloud vendor.")
	a.Describe(&s.Region, "The cloud region.")
	a.Describe(&s.Accelerator, "The accelerator type.")
	a.Describe(&s.InstanceType, "The instance family.")
	a.Describe(&s.InstanceSize, "The instance size.")
	a.Describe(&s.MinReplica, "Minimum replicas.")
	a.Describe(&s.MaxReplica, "Maximum replicas.")
	a.Describe(&s.ScaleToZeroTimeout, "Idle minutes before scale to zero, when set.")
	a.Describe(&s.Tags, "Labels attached to the endpoint.")
}

func summarizeEndpoint(ns string, d *EndpointDetails) InferenceEndpointSummary {
	return InferenceEndpointSummary{
		Name: d.Name, Namespace: ns, EndpointType: string(d.Type), Status: string(d.Status.State),
		StatusMessage: d.Status.Message, URL: d.Status.URL,
		ModelRepository: d.Model.Repository, ModelFramework: d.Model.Framework, ModelTask: d.Model.Task,
		ModelRevision: d.Model.Revision, Vendor: d.Provider.Vendor, Region: d.Provider.Region,
		Accelerator: d.Compute.Accelerator, InstanceType: d.Compute.InstanceType, InstanceSize: d.Compute.InstanceSize,
		MinReplica: d.Compute.Scaling.MinReplica, MaxReplica: d.Compute.Scaling.MaxReplica,
		ScaleToZeroTimeout: d.Compute.Scaling.ScaleToZeroTimeout, Tags: append([]string{}, d.Tags...),
	}
}

// GetInferenceEndpoint looks up an Inference Endpoint.
type GetInferenceEndpoint struct{}

// GetInferenceEndpointArgs selects the endpoint.
type GetInferenceEndpointArgs struct {
	Token     *string `pulumi:"token,optional" provider:"secret"`
	Namespace *string `pulumi:"namespace,optional"`
	Name      string  `pulumi:"name"`
}

func (f *GetInferenceEndpoint) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Inference Endpoint by namespace and name.")
}

func (a *GetInferenceEndpointArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.Namespace, "The owning user or organization. Defaults to the token's user.")
	ann.Describe(&a.Name, "The endpoint name.")
}

func (f *GetInferenceEndpoint) Invoke(ctx context.Context, req infer.FunctionRequest[GetInferenceEndpointArgs]) (infer.FunctionResponse[InferenceEndpointSummary], error) {
	out := infer.FunctionResponse[InferenceEndpointSummary]{}
	if req.Input.Name == "" {
		return out, fmt.Errorf("huggingface: getInferenceEndpoint needs a name")
	}
	client, token, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	ns, err := resolveNamespace(req.Input.Namespace, token, client)
	if err != nil {
		return out, fmt.Errorf("huggingface: resolve namespace: %w", err)
	}
	d, err := client.GetEndpoint(ns, req.Input.Name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up inference endpoint %q: %w", ns+"/"+req.Input.Name, err)
	}
	if d == nil {
		return out, fmt.Errorf("huggingface: inference endpoint %q was not found", ns+"/"+req.Input.Name)
	}
	out.Output = summarizeEndpoint(ns, d)
	return out, nil
}

// ListInferenceEndpoints lists the Inference Endpoints of a namespace.
type ListInferenceEndpoints struct{}

// ListInferenceEndpointsArgs selects the namespace.
type ListInferenceEndpointsArgs struct {
	Token     *string `pulumi:"token,optional" provider:"secret"`
	Namespace *string `pulumi:"namespace,optional"`
}

// ListInferenceEndpointsResult is the endpoint list.
type ListInferenceEndpointsResult struct {
	Namespace string                     `pulumi:"namespace"`
	Endpoints []InferenceEndpointSummary `pulumi:"endpoints"`
}

func (f *ListInferenceEndpoints) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the Hugging Face Inference Endpoints of a namespace.")
}

func (a *ListInferenceEndpointsArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.Namespace, "The user or organization to list. Defaults to the token's user.")
}

func (r *ListInferenceEndpointsResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.Namespace, "The namespace that was listed.")
	ann.Describe(&r.Endpoints, "The endpoints in the namespace.")
}

func (f *ListInferenceEndpoints) Invoke(ctx context.Context, req infer.FunctionRequest[ListInferenceEndpointsArgs]) (infer.FunctionResponse[ListInferenceEndpointsResult], error) {
	out := infer.FunctionResponse[ListInferenceEndpointsResult]{}
	client, token, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	ns, err := resolveNamespace(req.Input.Namespace, token, client)
	if err != nil {
		return out, fmt.Errorf("huggingface: resolve namespace: %w", err)
	}
	items, err := client.ListEndpoints(ns)
	if err != nil {
		return out, fmt.Errorf("huggingface: list inference endpoints of %q: %w", ns, err)
	}
	res := ListInferenceEndpointsResult{Namespace: ns, Endpoints: make([]InferenceEndpointSummary, 0, len(items))}
	for i := range items {
		res.Endpoints = append(res.Endpoints, summarizeEndpoint(ns, &items[i]))
	}
	out.Output = res
	return out, nil
}
