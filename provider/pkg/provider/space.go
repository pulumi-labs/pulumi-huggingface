package provider

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Space resource
// ---------------------------------------------------------------------------

// Space provisions a Hugging Face Space — an interactive ML demo or application
// hosted on Hugging Face infrastructure, backed by Gradio, Docker, or static HTML.
type Space struct{}

// SpaceArgs are the user-supplied inputs for a Space.
type SpaceArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`

	// Namespace is the Hugging Face user or organization that owns this Space.
	// Defaults to the user associated with the API token when not set.
	Namespace *string `pulumi:"namespace,optional"`

	// SDK is the frontend framework: "gradio", "docker", or "static".
	SDK string `pulumi:"sdk"`

	// Hardware is the compute tier for the Space. Defaults to "cpu-basic".
	// Options: "cpu-basic", "cpu-upgrade", "t4-small", "t4-medium",
	// "a10g-small", "a10g-large", "a100-large", "h100", "h100x8".
	Hardware *string `pulumi:"hardware,optional"`

	// Private controls whether the Space is private. Defaults to false (public).
	Private *bool `pulumi:"private,optional"`

	// SleepTimeSeconds is the idle duration before hibernation.
	// Set to -1 to disable sleep. Default is 3600 (1 hour).
	SleepTimeSeconds *int `pulumi:"sleepTimeSeconds,optional"`

	// Secrets are secret environment variables for the Space.
	// Values are write-only and not returned by the API on read.
	Secrets []SpaceSecretInput `pulumi:"secrets,optional"`

	// Variables are non-secret environment variables for the Space.
	Variables []SpaceVariableInput `pulumi:"variables,optional"`
}

// SpaceSecretInput is a secret key-value pair injected into the Space runtime.
type SpaceSecretInput struct {
	Key         string  `pulumi:"key"`
	Value       string  `pulumi:"value" provider:"secret"`
	Description *string `pulumi:"description,optional"`
}

// SpaceVariableInput is a non-secret key-value pair for the Space environment.
type SpaceVariableInput struct {
	Key         string  `pulumi:"key"`
	Value       string  `pulumi:"value"`
	Description *string `pulumi:"description,optional"`
}

// SpaceState is the persisted state of a Space.
type SpaceState struct {
	SpaceArgs

	// SpaceID is the canonical identifier: "namespace/name".
	SpaceID string `pulumi:"spaceId"`

	// URL is the public URL of the Space (e.g. "https://huggingface.co/spaces/user/my-space").
	URL string `pulumi:"url"`
}

func (r *Space) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Space — an interactive ML demo or application hosted "+
		"on Hugging Face infrastructure, backed by Gradio, Docker, or static HTML.")
}

func (a *SpaceArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.SDK, "The frontend framework: 'gradio', 'docker', or 'static'.")
	ann.Describe(&a.Hardware, "The compute tier (e.g. 'cpu-basic', 't4-small', 'a10g-small'). Defaults to 'cpu-basic'.")
	ann.Describe(&a.Secrets, "Secret environment variables injected at runtime. Values are write-only.")
	ann.Describe(&a.Variables, "Non-secret environment variables for the Space.")
}

func (s *SpaceState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.SpaceID, "The canonical Space identifier: 'namespace/name'.")
	ann.Describe(&s.URL, "The public URL of the Space.")
}

// Ensure Space implements all CRUD interfaces.
var _ infer.CustomResource[SpaceArgs, SpaceState] = (*Space)(nil)

func (r *Space) Create(
	ctx context.Context,
	req infer.CreateRequest[SpaceArgs],
) (infer.CreateResponse[SpaceState], error) {
	args := req.Inputs
	state := SpaceState{SpaceArgs: args}
	if req.DryRun {
		return infer.CreateResponse[SpaceState]{ID: req.Name, Output: state}, nil
	}

	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.CreateResponse[SpaceState]{}, fmt.Errorf("huggingface: API token is required")
	}
	client := newHFClient(token)

	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[SpaceState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	hardware := string(SpaceHardwareCPUBasic)
	if args.Hardware != nil {
		hardware = *args.Hardware
	}

	var secrets []SpaceSecret
	for _, s := range args.Secrets {
		desc := ""
		if s.Description != nil {
			desc = *s.Description
		}
		secrets = append(secrets, SpaceSecret{Key: s.Key, Value: s.Value, Description: desc})
	}
	var variables []SpaceEnvVar
	for _, v := range args.Variables {
		desc := ""
		if v.Description != nil {
			desc = *v.Description
		}
		variables = append(variables, SpaceEnvVar{Key: v.Key, Value: v.Value, Description: desc})
	}

	isPrivate := false
	if args.Private != nil {
		isPrivate = *args.Private
	}

	sdkStr := args.SDK
	createReq := CreateRepoRequest{
		Name:      req.Name,
		Type:      "space",
		Private:   &isPrivate,
		SDK:       &sdkStr,
		Hardware:  &hardware,
		Secrets:   secrets,
		Variables: variables,
	}
	if namespace != "" {
		createReq.Organization = &namespace
	}
	if args.SleepTimeSeconds != nil {
		createReq.SleepTime = args.SleepTimeSeconds
	}

	repo, err := client.CreateRepo(createReq)
	if err != nil {
		return infer.CreateResponse[SpaceState]{}, fmt.Errorf("create space %q: %w", req.Name, err)
	}

	id := repo.ID
	if id == "" {
		id = fmt.Sprintf("%s/%s", namespace, req.Name)
	}
	state.SpaceID = id
	state.URL = fmt.Sprintf("https://huggingface.co/spaces/%s", id)
	ns := namespace
	state.Namespace = &ns
	return infer.CreateResponse[SpaceState]{ID: id, Output: state}, nil
}

func (r *Space) Read(
	ctx context.Context,
	req infer.ReadRequest[SpaceArgs, SpaceState],
) (infer.ReadResponse[SpaceArgs, SpaceState], error) {
	args := req.Inputs
	state := req.State
	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.ReadResponse[SpaceArgs, SpaceState]{ID: req.ID, Inputs: args, State: state}, nil
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.ReadResponse[SpaceArgs, SpaceState]{}, err
	}

	repo, err := client.GetRepo("space", namespace, name)
	if err != nil {
		return infer.ReadResponse[SpaceArgs, SpaceState]{}, fmt.Errorf("read space %q: %w", req.ID, err)
	}
	if repo == nil {
		return infer.ReadResponse[SpaceArgs, SpaceState]{}, nil
	}

	state.SpaceID = req.ID
	state.URL = fmt.Sprintf("https://huggingface.co/spaces/%s", req.ID)
	private := repo.Private
	state.Private = &private
	if repo.Hardware != nil {
		state.Hardware = repo.Hardware
	}
	return infer.ReadResponse[SpaceArgs, SpaceState]{ID: req.ID, Inputs: args, State: state}, nil
}

func (r *Space) Update(
	ctx context.Context,
	req infer.UpdateRequest[SpaceArgs, SpaceState],
) (infer.UpdateResponse[SpaceState], error) {
	args := req.Inputs
	state := SpaceState{SpaceArgs: args}
	state.SpaceID = req.State.SpaceID
	state.URL = req.State.URL

	if req.DryRun {
		return infer.UpdateResponse[SpaceState]{Output: state}, nil
	}

	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.UpdateResponse[SpaceState]{}, fmt.Errorf("huggingface: API token is required")
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.UpdateResponse[SpaceState]{}, err
	}

	if args.Hardware != nil {
		if err := client.UpdateSpaceHardware(namespace, name, SpaceHardware(*args.Hardware)); err != nil {
			return infer.UpdateResponse[SpaceState]{}, fmt.Errorf("update space hardware: %w", err)
		}
	}

	if len(args.Secrets) > 0 {
		var secrets []SpaceSecret
		for _, s := range args.Secrets {
			desc := ""
			if s.Description != nil {
				desc = *s.Description
			}
			secrets = append(secrets, SpaceSecret{Key: s.Key, Value: s.Value, Description: desc})
		}
		if err := client.UpdateSpaceSecrets(namespace, name, secrets); err != nil {
			return infer.UpdateResponse[SpaceState]{}, fmt.Errorf("update space secrets: %w", err)
		}
	}

	if len(args.Variables) > 0 {
		var variables []SpaceEnvVar
		for _, v := range args.Variables {
			desc := ""
			if v.Description != nil {
				desc = *v.Description
			}
			variables = append(variables, SpaceEnvVar{Key: v.Key, Value: v.Value, Description: desc})
		}
		if err := client.UpdateSpaceVariables(namespace, name, variables); err != nil {
			return infer.UpdateResponse[SpaceState]{}, fmt.Errorf("update space variables: %w", err)
		}
	}

	return infer.UpdateResponse[SpaceState]{Output: state}, nil
}

func (r *Space) Delete(
	ctx context.Context,
	req infer.DeleteRequest[SpaceState],
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
	if err := client.DeleteRepo("space", namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete space %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
