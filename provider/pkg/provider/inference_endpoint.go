package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// InferenceEndpoint resource
// ---------------------------------------------------------------------------

// InferenceEndpoint provisions a Hugging Face Inference Endpoint — a dedicated,
// auto-scalable compute instance that serves a model for production inference traffic.
// Supports public, protected, and private access types across AWS, Azure, and GCP.
type InferenceEndpoint struct{}

// InferenceEndpointArgs are the user-supplied inputs for an inference endpoint.
type InferenceEndpointArgs struct {
	// Token authenticates with the Hugging Face API. Falls back to the
	// provider-level token when not set.
	Token *string `pulumi:"token,optional" provider:"secret"`

	// Namespace is the Hugging Face user or organization that owns this endpoint.
	// Defaults to the user associated with the API token when not set.
	Namespace *string `pulumi:"namespace,optional"`

	// ModelRepository is the ID of the model to serve (e.g. "meta-llama/Llama-3-8B-Instruct").
	ModelRepository string `pulumi:"modelRepository"`

	// ModelFramework is the serving framework (e.g. "pytorch", "custom").
	// When using TGI/TEI/vLLM, set to "pytorch" or "custom" and supply CustomImageURL.
	ModelFramework string `pulumi:"modelFramework"`

	// ModelTask is the inference task type (e.g. "text-generation", "feature-extraction").
	ModelTask string `pulumi:"modelTask"`

	// ModelRevision pins the model to a specific git commit SHA or branch.
	// Defaults to the latest main branch revision when not set.
	ModelRevision *string `pulumi:"modelRevision,optional"`

	// CustomImageURL allows deploying a custom container image (e.g. TGI, TEI, vLLM).
	// Example: "ghcr.io/huggingface/text-generation-inference:2.4.0"
	CustomImageURL *string `pulumi:"customImageUrl,optional"`

	// CustomImageEnvVars sets container environment variables when using a custom image.
	CustomImageEnvVars map[string]string `pulumi:"customImageEnvVars,optional"`

	// CustomImagePort overrides the default container port (8080).
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
	// Only applies when MinReplica is 0. Defaults to 60 minutes.
	ScaleToZeroTimeout *int `pulumi:"scaleToZeroTimeout,optional"`

	// EndpointType controls access: "public", "protected" (default), or "private".
	// Protected endpoints require a Hugging Face account. Private endpoints require
	// VPC PrivateLink configuration (set AccountID).
	EndpointType *string `pulumi:"endpointType,optional"`

	// AccountID is required for private endpoints using AWS VPC PrivateLink.
	AccountID *string `pulumi:"accountId,optional"`
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
		"instance that serves a model for production inference traffic.")
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
}

func (s *InferenceEndpointState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.URL, "The HTTPS URL to call for inference. Only set when the endpoint is in 'running' state.")
	ann.Describe(&s.Status, "The current lifecycle state: 'pending', 'initializing', 'running', 'paused', 'scaledToZero', 'failed'.")
}

// Ensure InferenceEndpoint implements the CustomResource interfaces.
var _ infer.CustomResource[InferenceEndpointArgs, InferenceEndpointState] = (*InferenceEndpoint)(nil)

func buildEndpointCreateRequest(name string, args InferenceEndpointArgs) CreateEndpointRequest {
	minR := 1
	if args.MinReplica != nil {
		minR = *args.MinReplica
	}
	maxR := 1
	if args.MaxReplica != nil {
		maxR = *args.MaxReplica
	}
	endType := EndpointTypeProtected
	if args.EndpointType != nil {
		endType = EndpointType(*args.EndpointType)
	}

	scaling := EndpointScaling{
		MinReplica:         minR,
		MaxReplica:         maxR,
		ScaleToZeroTimeout: args.ScaleToZeroTimeout,
	}

	var img *EndpointImage
	if args.CustomImageURL != nil {
		custom := &EndpointCustomImage{
			URL:     *args.CustomImageURL,
			EnvVars: args.CustomImageEnvVars,
			Port:    args.CustomImagePort,
		}
		if args.CustomImageHealthRoute != nil {
			custom.Health = &ImageHealth{Route: *args.CustomImageHealthRoute}
		}
		img = &EndpointImage{Custom: custom}
	}

	return CreateEndpointRequest{
		Name:      name,
		Type:      endType,
		AccountID: args.AccountID,
		Compute: EndpointCompute{
			Accelerator:  args.Accelerator,
			InstanceType: args.InstanceType,
			InstanceSize: args.InstanceSize,
			Scaling:      scaling,
		},
		Model: EndpointModel{
			Repository: args.ModelRepository,
			Framework:  args.ModelFramework,
			Task:       args.ModelTask,
			Revision:   args.ModelRevision,
			Image:      img,
		},
		Provider: EndpointProvider{
			Vendor: args.Vendor,
			Region: args.Region,
		},
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

	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.CreateResponse[InferenceEndpointState]{}, fmt.Errorf("huggingface: API token is required (set token input or HUGGINGFACE_TOKEN env var)")
	}
	client := newHFClient(token)

	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[InferenceEndpointState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	endpointReq := buildEndpointCreateRequest(req.Name, args)
	endpoint, err := client.CreateEndpoint(namespace, endpointReq)
	if err != nil {
		return infer.CreateResponse[InferenceEndpointState]{}, fmt.Errorf("create inference endpoint %q: %w", req.Name, err)
	}

	id := fmt.Sprintf("%s/%s", namespace, req.Name)
	state.URL = endpoint.Status.URL
	state.Status = string(endpoint.Status.State)
	ns := namespace
	state.Namespace = &ns
	return infer.CreateResponse[InferenceEndpointState]{ID: id, Output: state}, nil
}

func (r *InferenceEndpoint) Read(
	ctx context.Context,
	req infer.ReadRequest[InferenceEndpointArgs, InferenceEndpointState],
) (infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState], error) {
	args := req.Inputs
	state := req.State
	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]{
			ID: req.ID, Inputs: args, State: state,
		}, nil
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]{}, err
	}

	endpoint, err := client.GetEndpoint(namespace, name)
	if err != nil {
		return infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]{}, fmt.Errorf("read inference endpoint %q: %w", req.ID, err)
	}
	if endpoint == nil {
		// Resource has been deleted externally — signal removal by returning empty ID.
		return infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]{}, nil
	}

	state.URL = endpoint.Status.URL
	state.Status = string(endpoint.Status.State)
	ns := endpoint.Namespace
	state.Namespace = &ns
	return infer.ReadResponse[InferenceEndpointArgs, InferenceEndpointState]{
		ID: req.ID, Inputs: args, State: state,
	}, nil
}

func (r *InferenceEndpoint) Update(
	ctx context.Context,
	req infer.UpdateRequest[InferenceEndpointArgs, InferenceEndpointState],
) (infer.UpdateResponse[InferenceEndpointState], error) {
	args := req.Inputs
	state := InferenceEndpointState{InferenceEndpointArgs: args}
	if req.DryRun {
		state.URL = req.State.URL
		state.Status = req.State.Status
		return infer.UpdateResponse[InferenceEndpointState]{Output: state}, nil
	}

	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.UpdateResponse[InferenceEndpointState]{}, fmt.Errorf("huggingface: API token is required")
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.UpdateResponse[InferenceEndpointState]{}, err
	}

	endType := EndpointTypeProtected
	if args.EndpointType != nil {
		endType = EndpointType(*args.EndpointType)
	}
	minR := 1
	if args.MinReplica != nil {
		minR = *args.MinReplica
	}
	maxR := 1
	if args.MaxReplica != nil {
		maxR = *args.MaxReplica
	}
	compute := EndpointCompute{
		Accelerator:  args.Accelerator,
		InstanceType: args.InstanceType,
		InstanceSize: args.InstanceSize,
		Scaling: EndpointScaling{
			MinReplica:         minR,
			MaxReplica:         maxR,
			ScaleToZeroTimeout: args.ScaleToZeroTimeout,
		},
	}
	updateReq := UpdateEndpointRequest{
		Type:    &endType,
		Compute: &compute,
	}

	endpoint, err := client.UpdateEndpoint(namespace, name, updateReq)
	if err != nil {
		return infer.UpdateResponse[InferenceEndpointState]{}, fmt.Errorf("update inference endpoint %q: %w", req.ID, err)
	}
	state.URL = endpoint.Status.URL
	state.Status = string(endpoint.Status.State)
	return infer.UpdateResponse[InferenceEndpointState]{Output: state}, nil
}

func (r *InferenceEndpoint) Delete(
	ctx context.Context,
	req infer.DeleteRequest[InferenceEndpointState],
) (infer.DeleteResponse, error) {
	state := req.State
	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(state.Token, cfg.Token)
	if token == "" {
		return infer.DeleteResponse{}, fmt.Errorf("huggingface: API token is required")
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteEndpoint(namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete inference endpoint %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func resolveToken(resourceToken *string, cfgToken *string) string {
	if resourceToken != nil && *resourceToken != "" {
		return *resourceToken
	}
	if cfgToken != nil && *cfgToken != "" {
		return *cfgToken
	}
	return ""
}

func resolveNamespace(ns *string, token string, client *hfClient) (string, error) {
	if ns != nil && *ns != "" {
		return *ns, nil
	}
	// Whoami call to get the authenticated user's namespace.
	body, _, err := client.do("GET", hubBase+"/api/whoami-v2", nil)
	if err != nil {
		return "", fmt.Errorf("fetch authenticated user namespace: %w", err)
	}
	var who struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &who); err != nil || who.Name == "" {
		return "", fmt.Errorf("could not determine namespace from token; set namespace explicitly")
	}
	return who.Name, nil
}

// SplitID parses a "namespace/name" resource ID into its components.
// Exported for use in tests.
func SplitID(id string) (namespace, name string, err error) {
	return splitID(id)
}

func splitID(id string) (namespace, name string, err error) {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' {
			return id[:i], id[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid resource ID %q: expected namespace/name", id)
}

func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
