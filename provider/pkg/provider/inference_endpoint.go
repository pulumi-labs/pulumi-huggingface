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
	"time"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// InferenceEndpoint resource
// ---------------------------------------------------------------------------

// InferenceEndpoint provisions a Hugging Face Inference Endpoint — a dedicated,
// auto-scalable compute instance that serves a model for production inference traffic.
// Supports public, authenticated, and private access types across AWS, Azure, and GCP.
type InferenceEndpoint struct{}

// Endpoint wait tuning. Variables so tests can run with short intervals.
var (
	endpointPollInterval = 10 * time.Second
	endpointWaitTimeout  = 30 * time.Minute
)

// defaultEndpointType is the access type used when endpointType is unset.
const defaultEndpointType = string(EndpointTypeAuthenticated)

// InferenceEndpointArgs are the user-supplied inputs for an inference endpoint.
type InferenceEndpointArgs struct {
	// Token authenticates with the Hugging Face API. Falls back to the
	// provider-level token when not set.
	Token *string `pulumi:"token,optional" provider:"secret"`

	// Namespace is the Hugging Face user or organization that owns this endpoint.
	// Defaults to the user associated with the API token when not set. Changing
	// it replaces the endpoint.
	Namespace *string `pulumi:"namespace,optional" provider:"replaceOnChanges"`

	// ModelRepository is the ID of the model to serve (e.g. "meta-llama/Llama-3-8B-Instruct").
	ModelRepository string `pulumi:"modelRepository"`

	// ModelFramework is the serving framework (e.g. "pytorch", "custom").
	ModelFramework string `pulumi:"modelFramework"`

	// ModelTask is the inference task type (e.g. "text-generation", "feature-extraction").
	ModelTask string `pulumi:"modelTask"`

	// ModelRevision pins the model to a specific git commit SHA or branch.
	ModelRevision *string `pulumi:"modelRevision,optional"`

	// CustomImageURL allows deploying a custom container image.
	// Example: "ghcr.io/huggingface/text-generation-inference:2.4.0"
	CustomImageURL *string `pulumi:"customImageUrl,optional"`

	// CustomImageEnvVars sets container environment variables (model.env).
	CustomImageEnvVars map[string]string `pulumi:"customImageEnvVars,optional"`

	// CustomImagePort overrides the default container port.
	CustomImagePort *int `pulumi:"customImagePort,optional"`

	// CustomImageHealthRoute is the HTTP path for the health check (e.g. "/health").
	CustomImageHealthRoute *string `pulumi:"customImageHealthRoute,optional"`

	// Vendor is the cloud provider: "aws", "azure", or "gcp".
	Vendor string `pulumi:"vendor"`

	// Region is the cloud region for the endpoint (e.g. "us-east-1" for AWS).
	Region string `pulumi:"region"`

	// Accelerator is the hardware accelerator type: "cpu", "gpu", or "inf2".
	Accelerator string `pulumi:"accelerator"`

	// InstanceType is the specific instance family (e.g. "nvidia-a10g", "intel-icl").
	InstanceType string `pulumi:"instanceType"`

	// InstanceSize is the instance size multiplier (e.g. "x1", "x2", "x4").
	InstanceSize string `pulumi:"instanceSize"`

	// MinReplica is the minimum number of replicas. Set to 0 to enable scale-to-zero.
	// Defaults to 1.
	MinReplica *int `pulumi:"minReplica,optional"`

	// MaxReplica is the maximum number of replicas for autoscaling. Defaults to 1.
	MaxReplica *int `pulumi:"maxReplica,optional"`

	// ScaleToZeroTimeout is the idle time in minutes before scaling to zero.
	// Only applies when MinReplica is 0. The API default is 15 minutes.
	ScaleToZeroTimeout *int `pulumi:"scaleToZeroTimeout,optional"`

	// EndpointType controls access: "public", "authenticated" (default), or "private".
	EndpointType *string `pulumi:"endpointType,optional"`

	// AccountID is the AWS account ID for private endpoints using AWS PrivateLink
	// (sent as privateService.accountId). Changing it replaces the endpoint.
	AccountID *string `pulumi:"accountId,optional" provider:"replaceOnChanges"`

	// PrivateLinkRegion is the AWS region of the PrivateLink entry point
	// ("us-east-1" or "eu-west-1"), sent as privateService.region. Defaults to
	// region when accountId is set. Changing it replaces the endpoint.
	PrivateLinkRegion *string `pulumi:"privateLinkRegion,optional" provider:"replaceOnChanges"`

	// Tags are free-form labels attached to the endpoint (the API's tags
	// field). Unmanaged while unset; removing tags that were set clears them.
	Tags []string `pulumi:"tags,optional"`
}

// InferenceEndpointState is the persisted state of an inference endpoint.
type InferenceEndpointState struct {
	InferenceEndpointArgs

	// URL is the HTTPS endpoint URL. Only populated when the endpoint is running.
	URL string `pulumi:"url"`

	// Status is the current lifecycle state of the endpoint.
	Status string `pulumi:"status"`
}

func (r *InferenceEndpoint) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Inference Endpoint — a dedicated, auto-scalable compute "+
		"instance that serves a model for production inference traffic. "+
		"Import with the ID 'namespace/name'.")
}

func (a *InferenceEndpointArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.ModelRepository, "The Hugging Face model repository ID (e.g. 'meta-llama/Llama-3-8B-Instruct').")
	ann.Describe(&a.ModelFramework, "The serving framework (e.g. 'pytorch', 'custom'). Use 'pytorch' for Transformers, 'custom' for TGI/TEI/vLLM.")
	ann.Describe(&a.ModelTask, "The inference task type (e.g. 'text-generation', 'feature-extraction', 'sentence-similarity').")
	ann.Describe(&a.Vendor, "Cloud vendor: 'aws', 'azure', or 'gcp'.")
	ann.Describe(&a.Region, "Cloud region (e.g. 'us-east-1' for AWS, 'eastus' for Azure, 'us-central1' for GCP).")
	ann.Describe(&a.Accelerator, "Hardware accelerator type: 'cpu', 'gpu', or 'inf2' (AWS Inferentia 2).")
	ann.Describe(&a.InstanceType, "Instance family (e.g. 'nvidia-a10g', 'nvidia-h100', 'intel-icl').")
	ann.Describe(&a.InstanceSize, "Instance size multiplier: 'x1', 'x2', 'x4', 'x8'.")
	ann.Describe(&a.EndpointType, "Access type: 'public', 'authenticated' (default), or 'private'. "+
		"The legacy 'protected' type is no longer supported by the API.")
	ann.Describe(&a.ScaleToZeroTimeout, "Idle minutes before scaling to zero when minReplica is 0. The API default is 15.")
	ann.Describe(&a.CustomImageEnvVars, "Environment variables for the container (model.env).")
	ann.Describe(&a.Tags, "Labels attached to the endpoint. Unmanaged while unset; removing tags that were set clears them.")
	ann.Describe(&a.AccountID, "AWS account ID for AWS PrivateLink on a private endpoint. Changing it replaces the endpoint.")
	ann.Describe(&a.PrivateLinkRegion, "AWS region of the PrivateLink entry point ('us-east-1' or 'eu-west-1'). Defaults to region.")
}

func (s *InferenceEndpointState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.URL, "The HTTPS URL to call for inference. Set once the endpoint is running.")
	ann.Describe(&s.Status, "The current lifecycle state: 'pending', 'initializing', 'updating', 'running', 'paused', 'scaledToZero', 'failed', 'updateFailed'.")
}

// Ensure InferenceEndpoint implements the CustomResource interfaces.
var (
	_ infer.CustomResource[InferenceEndpointArgs, InferenceEndpointState] = (*InferenceEndpoint)(nil)
	_ infer.CustomCheck[InferenceEndpointArgs]                            = (*InferenceEndpoint)(nil)
	_ infer.CustomDiff[InferenceEndpointArgs, InferenceEndpointState]     = (*InferenceEndpoint)(nil)
	_ infer.CustomRead[InferenceEndpointArgs, InferenceEndpointState]     = (*InferenceEndpoint)(nil)
	_ infer.CustomUpdate[InferenceEndpointArgs, InferenceEndpointState]   = (*InferenceEndpoint)(nil)
	_ infer.CustomDelete[InferenceEndpointState]                          = (*InferenceEndpoint)(nil)
)

func (r *InferenceEndpoint) Check(
	ctx context.Context,
	req infer.CheckRequest,
) (infer.CheckResponse[InferenceEndpointArgs], error) {
	args, failures, err := infer.DefaultCheck[InferenceEndpointArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[InferenceEndpointArgs]{Inputs: args, Failures: failures}, err
	}
	if args.EndpointType != nil {
		switch EndpointType(*args.EndpointType) {
		case EndpointTypePublic, EndpointTypeAuthenticated, EndpointTypePrivate:
		case "protected":
			failures = append(failures, p.CheckFailure{
				Property: "endpointType",
				Reason:   "'protected' is no longer supported by the Inference Endpoints API; use 'authenticated'",
			})
		default:
			failures = append(failures, p.CheckFailure{
				Property: "endpointType",
				Reason:   fmt.Sprintf("invalid endpoint type %q; must be 'public', 'authenticated' or 'private'", *args.EndpointType),
			})
		}
	}
	return infer.CheckResponse[InferenceEndpointArgs]{Inputs: args, Failures: failures}, nil
}

// endpointImage builds the model.image union. The API requires it: a custom
// container when customImageUrl is set, the stock Hugging Face image otherwise.
func endpointImage(args InferenceEndpointArgs) *EndpointImage {
	if args.CustomImageURL != nil && *args.CustomImageURL != "" {
		return &EndpointImage{Custom: &EndpointCustomImage{
			URL:         *args.CustomImageURL,
			Port:        args.CustomImagePort,
			HealthRoute: args.CustomImageHealthRoute,
		}}
	}
	return &EndpointImage{Huggingface: &struct{}{}}
}

func endpointCompute(args InferenceEndpointArgs) EndpointCompute {
	return EndpointCompute{
		Accelerator:  args.Accelerator,
		InstanceType: args.InstanceType,
		InstanceSize: args.InstanceSize,
		Scaling: EndpointScaling{
			MinReplica:         orDefault(args.MinReplica, 1),
			MaxReplica:         orDefault(args.MaxReplica, 1),
			ScaleToZeroTimeout: args.ScaleToZeroTimeout,
		},
	}
}

func buildEndpointCreateRequest(name string, args InferenceEndpointArgs) CreateEndpointRequest {
	model := EndpointModel{
		Repository: args.ModelRepository,
		Framework:  args.ModelFramework,
		Task:       args.ModelTask,
		Revision:   args.ModelRevision,
		Image:      endpointImage(args),
	}
	if len(args.CustomImageEnvVars) > 0 {
		env := maps.Clone(args.CustomImageEnvVars)
		model.Env = &env
	}
	req := CreateEndpointRequest{
		Name:     name,
		Type:     EndpointType(orDefault(args.EndpointType, defaultEndpointType)),
		Compute:  endpointCompute(args),
		Model:    model,
		Provider: EndpointProvider{Vendor: args.Vendor, Region: args.Region},
		Tags:     slices.Clone(args.Tags),
	}
	if args.AccountID != nil && *args.AccountID != "" {
		req.PrivateService = &PrivateService{
			AccountID: *args.AccountID,
			Region:    orDefault(args.PrivateLinkRegion, args.Region),
		}
	}
	return req
}

// buildEndpointUpdateRequest sends every property the API can update. The
// image is only sent when it changed, so an image configured outside Pulumi
// (for example a managed engine variant) is not clobbered by unrelated edits.
func buildEndpointUpdateRequest(old, args InferenceEndpointArgs) UpdateEndpointRequest {
	endType := EndpointType(orDefault(args.EndpointType, defaultEndpointType))
	compute := endpointCompute(args)
	model := EndpointModel{
		Repository: args.ModelRepository,
		Framework:  args.ModelFramework,
		Task:       args.ModelTask,
		Revision:   args.ModelRevision,
	}
	if len(args.CustomImageEnvVars) > 0 || len(old.CustomImageEnvVars) > 0 {
		env := maps.Clone(args.CustomImageEnvVars)
		if env == nil {
			env = map[string]string{}
		}
		model.Env = &env
	}
	if imageChanged(old, args) {
		model.Image = endpointImage(args)
	}
	upd := UpdateEndpointRequest{
		Type:     &endType,
		Compute:  &compute,
		Model:    &model,
		Provider: &EndpointProvider{Vendor: args.Vendor, Region: args.Region},
	}
	if tagsChanged(old.Tags, args.Tags) {
		tags := slices.Clone(args.Tags)
		if tags == nil {
			tags = []string{}
		}
		upd.Tags = &tags
	}
	return upd
}

// tagsChanged reports a change in the managed tags. An empty or unset list
// manages nothing, except that removing tags that were set clears them (an
// empty list and an unset one cannot be told apart once decoded).
func tagsChanged(old, new []string) bool {
	if len(new) == 0 {
		return len(old) > 0
	}
	return !tagsEqual(old, new)
}

// tagsEqual compares tag lists ignoring order: the API may reorder them.
func tagsEqual(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func imageChanged(old, n InferenceEndpointArgs) bool {
	return orDefault(old.CustomImageURL, "") != orDefault(n.CustomImageURL, "") ||
		ptrChanged(old.CustomImagePort, n.CustomImagePort) ||
		ptrChanged(old.CustomImageHealthRoute, n.CustomImageHealthRoute)
}

// waitForEndpoint polls until the endpoint leaves its transitional states, so
// outputs such as url are populated when the resource operation returns.
func waitForEndpoint(ctx context.Context, c *hfClient, ns, name string, d *EndpointDetails) (*EndpointDetails, error) {
	deadline := time.Now().Add(endpointWaitTimeout)
	for {
		switch d.Status.State {
		case EndpointStateRunning, EndpointStateScaledToZero, EndpointStatePaused:
			return d, nil
		case EndpointStateFailed, EndpointStateUpdateFailed:
			return d, fmt.Errorf("inference endpoint %s/%s is %s: %s", ns, name, d.Status.State, d.Status.Message)
		}
		if time.Now().After(deadline) {
			return d, fmt.Errorf("timed out waiting for inference endpoint %s/%s (state %q)", ns, name, d.Status.State)
		}
		select {
		case <-ctx.Done():
			return d, ctx.Err()
		case <-time.After(endpointPollInterval):
		}
		nd, err := c.GetEndpoint(ns, name)
		if err != nil {
			return d, err
		}
		if nd == nil {
			return d, fmt.Errorf("inference endpoint %s/%s disappeared while waiting", ns, name)
		}
		d = nd
	}
}

func (r *InferenceEndpoint) Create(
	ctx context.Context,
	req infer.CreateRequest[InferenceEndpointArgs],
) (infer.CreateResponse[InferenceEndpointState], error) {
	args := req.Inputs
	state := InferenceEndpointState{InferenceEndpointArgs: args}
	if req.DryRun {
		return infer.CreateResponse[InferenceEndpointState]{ID: req.Name, Output: state}, nil
	}

	client, token, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[InferenceEndpointState]{}, err
	}
	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[InferenceEndpointState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	endpoint, err := client.CreateEndpoint(namespace, buildEndpointCreateRequest(req.Name, args))
	if err != nil {
		return infer.CreateResponse[InferenceEndpointState]{}, fmt.Errorf("create inference endpoint %q: %w", req.Name, err)
	}

	id := fmt.Sprintf("%s/%s", namespace, req.Name)
	state.Namespace = ptr(namespace)
	endpoint, waitErr := waitForEndpoint(ctx, client, namespace, req.Name, endpoint)
	state.URL = endpoint.Status.URL
	state.Status = string(endpoint.Status.State)
	if waitErr != nil {
		// The endpoint exists; record it so the next refresh or update can fix it.
		return infer.CreateResponse[InferenceEndpointState]{ID: id, Output: state},
			infer.ResourceInitFailedError{Reasons: []string{waitErr.Error()}}
	}
	return infer.CreateResponse[InferenceEndpointState]{ID: id, Output: state}, nil
}

// refreshEndpoint maps an API response onto inputs and state. State is the
// API's truth. Inputs keep the user's intent: an optional input that is unset
// stays unset unless the API drifted from its default, while an import (no
// prior inputs) adopts everything the API reports.
func refreshEndpoint(ns string, prior InferenceEndpointArgs, d *EndpointDetails) (inputs, state InferenceEndpointArgs) {
	imported := prior.ModelRepository == ""

	apiType := string(d.Type)
	var custom *EndpointCustomImage
	if d.Model.Image != nil {
		custom = d.Model.Image.Custom
	}
	var apiEnv map[string]string
	if d.Model.Env != nil {
		apiEnv = *d.Model.Env
	}
	defType := defaultEndpointType
	one := 1

	inputs = InferenceEndpointArgs{
		Token:           prior.Token,
		Namespace:       prior.Namespace,
		ModelRepository: d.Model.Repository,
		ModelFramework:  d.Model.Framework,
		ModelTask:       d.Model.Task,
		Vendor:          d.Provider.Vendor,
		Region:          d.Provider.Region,
		Accelerator:     d.Compute.Accelerator,
		InstanceType:    d.Compute.InstanceType,
		InstanceSize:    d.Compute.InstanceSize,
	}
	if imported {
		inputs.Namespace = ptr(ns)
	}
	inputs.ModelRevision = refreshOpt(prior.ModelRevision, orDefault(d.Model.Revision, ""), d.Model.Revision != nil, imported, nil)
	inputs.EndpointType = refreshOpt(prior.EndpointType, apiType, apiType != "", imported, &defType)
	inputs.MinReplica = refreshOpt(prior.MinReplica, d.Compute.Scaling.MinReplica, true, imported, &one)
	inputs.MaxReplica = refreshOpt(prior.MaxReplica, d.Compute.Scaling.MaxReplica, true, imported, &one)
	inputs.ScaleToZeroTimeout = refreshOpt(prior.ScaleToZeroTimeout,
		orDefault(d.Compute.Scaling.ScaleToZeroTimeout, 0), d.Compute.Scaling.ScaleToZeroTimeout != nil, imported, nil)

	if custom != nil {
		inputs.CustomImageURL = refreshOpt(prior.CustomImageURL, custom.URL, custom.URL != "", imported, nil)
		inputs.CustomImagePort = refreshOpt(prior.CustomImagePort, orDefault(custom.Port, 0), custom.Port != nil, imported, nil)
		inputs.CustomImageHealthRoute = refreshOpt(prior.CustomImageHealthRoute,
			orDefault(custom.HealthRoute, ""), custom.HealthRoute != nil, imported, nil)
	} else if prior.CustomImageURL != nil && *prior.CustomImageURL != "" {
		// The API no longer runs the custom image: surface the drift by
		// dropping the input so the next update restores it.
		inputs.CustomImageURL = nil
	} else {
		inputs.CustomImagePort = prior.CustomImagePort
		inputs.CustomImageHealthRoute = prior.CustomImageHealthRoute
	}

	// Environment: on refresh only the keys the user manages are compared, so
	// variables injected by the platform do not cause a permanent diff.
	if imported {
		if len(apiEnv) > 0 {
			inputs.CustomImageEnvVars = maps.Clone(apiEnv)
		}
	} else if len(prior.CustomImageEnvVars) > 0 {
		env := map[string]string{}
		for k := range prior.CustomImageEnvVars {
			if v, ok := apiEnv[k]; ok {
				env[k] = v
			}
		}
		inputs.CustomImageEnvVars = env
	}

	// Tags: unmanaged while unset (an unset input never diffs); once managed
	// the API's list is the truth, keeping the user's order when equal.
	if imported {
		if len(d.Tags) > 0 {
			inputs.Tags = slices.Clone(d.Tags)
		}
	} else if len(prior.Tags) > 0 {
		if tagsEqual(prior.Tags, d.Tags) {
			inputs.Tags = prior.Tags
		} else {
			inputs.Tags = append([]string{}, d.Tags...)
		}
	}

	if ps := d.PrivateService; ps != nil && ps.AccountID != "" {
		inputs.AccountID = refreshOpt(prior.AccountID, ps.AccountID, true, imported, nil)
		inputs.PrivateLinkRegion = refreshOpt(prior.PrivateLinkRegion, ps.Region, ps.Region != "", imported, nil)
	} else {
		inputs.AccountID = prior.AccountID
		inputs.PrivateLinkRegion = prior.PrivateLinkRegion
	}

	state = inputs
	state.Namespace = ptr(ns)
	return inputs, state
}

func (r *InferenceEndpoint) Read(
	ctx context.Context,
	req infer.ReadRequest[InferenceEndpointArgs, InferenceEndpointState],
) (infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState], error) {
	type resp = infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	namespace, name, err := splitID(req.ID)
	if err != nil {
		return resp{}, err
	}
	endpoint, err := client.GetEndpoint(namespace, name)
	if err != nil {
		return resp{}, fmt.Errorf("read inference endpoint %q: %w", req.ID, err)
	}
	if endpoint == nil {
		// Deleted outside Pulumi: an empty ID drops the resource from state.
		return resp{}, nil
	}

	inputs, stateArgs := refreshEndpoint(namespace, prior, endpoint)
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State: InferenceEndpointState{
			InferenceEndpointArgs: stateArgs,
			URL:                   endpoint.Status.URL,
			Status:                string(endpoint.Status.State),
		},
	}, nil
}

func (r *InferenceEndpoint) Diff(
	_ context.Context,
	req infer.DiffRequest[InferenceEndpointArgs, InferenceEndpointState],
) (infer.DiffResponse, error) {
	o, n := req.State.InferenceEndpointArgs, req.Inputs
	d := newDiffSet()

	idNS, _, _ := splitID(req.ID)
	d.replace("namespace", namespaceChanged(idNS, n.Namespace))
	d.replace("accountId", orDefault(o.AccountID, "") != orDefault(n.AccountID, ""))
	d.replace("privateLinkRegion", ptrChanged(o.PrivateLinkRegion, n.PrivateLinkRegion))

	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("modelRepository", o.ModelRepository != n.ModelRepository)
	d.update("modelFramework", o.ModelFramework != n.ModelFramework)
	d.update("modelTask", o.ModelTask != n.ModelTask)
	d.update("modelRevision", ptrChanged(o.ModelRevision, n.ModelRevision))
	d.update("customImageUrl", orDefault(o.CustomImageURL, "") != orDefault(n.CustomImageURL, ""))
	d.update("customImageEnvVars", !maps.Equal(o.CustomImageEnvVars, n.CustomImageEnvVars))
	d.update("customImagePort", ptrChanged(o.CustomImagePort, n.CustomImagePort))
	d.update("customImageHealthRoute", ptrChanged(o.CustomImageHealthRoute, n.CustomImageHealthRoute))
	d.update("vendor", o.Vendor != n.Vendor)
	d.update("region", o.Region != n.Region)
	d.update("accelerator", o.Accelerator != n.Accelerator)
	d.update("instanceType", o.InstanceType != n.InstanceType)
	d.update("instanceSize", o.InstanceSize != n.InstanceSize)
	d.update("minReplica", orDefault(o.MinReplica, 1) != orDefault(n.MinReplica, 1))
	d.update("maxReplica", orDefault(o.MaxReplica, 1) != orDefault(n.MaxReplica, 1))
	d.update("scaleToZeroTimeout", ptrChanged(o.ScaleToZeroTimeout, n.ScaleToZeroTimeout))
	d.update("tags", tagsChanged(o.Tags, n.Tags))
	d.update("endpointType",
		orDefault(o.EndpointType, defaultEndpointType) != orDefault(n.EndpointType, defaultEndpointType))
	return d.response(), nil
}

func (r *InferenceEndpoint) Update(
	ctx context.Context,
	req infer.UpdateRequest[InferenceEndpointArgs, InferenceEndpointState],
) (infer.UpdateResponse[InferenceEndpointState], error) {
	args := req.Inputs
	state := InferenceEndpointState{InferenceEndpointArgs: args}
	ns, name, err := splitID(req.ID)
	if err != nil {
		return infer.UpdateResponse[InferenceEndpointState]{}, err
	}
	state.Namespace = ptr(ns)
	if req.DryRun {
		state.URL = req.State.URL
		state.Status = req.State.Status
		return infer.UpdateResponse[InferenceEndpointState]{Output: state}, nil
	}

	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.UpdateResponse[InferenceEndpointState]{}, err
	}

	endpoint, err := client.UpdateEndpoint(ns, name, buildEndpointUpdateRequest(req.State.InferenceEndpointArgs, args))
	if err != nil {
		return infer.UpdateResponse[InferenceEndpointState]{}, fmt.Errorf("update inference endpoint %q: %w", req.ID, err)
	}
	endpoint, waitErr := waitForEndpoint(ctx, client, ns, name, endpoint)
	state.URL = endpoint.Status.URL
	state.Status = string(endpoint.Status.State)
	if waitErr != nil {
		return infer.UpdateResponse[InferenceEndpointState]{Output: state},
			infer.ResourceInitFailedError{Reasons: []string{waitErr.Error()}}
	}
	return infer.UpdateResponse[InferenceEndpointState]{Output: state}, nil
}

func (r *InferenceEndpoint) Delete(
	ctx context.Context,
	req infer.DeleteRequest[InferenceEndpointState],
) (infer.DeleteResponse, error) {
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteEndpoint(namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete inference endpoint %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
