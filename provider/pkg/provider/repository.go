package provider

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Repository resource
// ---------------------------------------------------------------------------

// Repository provisions a Hugging Face Hub repository — a model repo or dataset
// repo. This resource manages the repository container and visibility, not the
// files within it. For Spaces, use the Space resource instead.
type Repository struct{}

// RepositoryArgs are the user-supplied inputs for a repository.
type RepositoryArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`

	// Namespace is the Hugging Face user or organization that owns this repository.
	// Defaults to the user associated with the API token when not set.
	Namespace *string `pulumi:"namespace,optional"`

	// RepoType is the repository type: "model" (default) or "dataset".
	RepoType *string `pulumi:"repoType,optional"`

	// Private controls whether the repository is private. Defaults to false.
	Private *bool `pulumi:"private,optional"`
}

// RepositoryState is the persisted state of a repository.
type RepositoryState struct {
	RepositoryArgs

	// RepoID is the canonical repository identifier: "namespace/name".
	RepoID string `pulumi:"repoId"`

	// URL is the web URL of the repository.
	URL string `pulumi:"url"`
}

func (r *Repository) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Hub repository (model or dataset). Manages the repository "+
		"container and visibility — not the files within it.")
}

func (a *RepositoryArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.RepoType, "The repository type: 'model' (default) or 'dataset'.")
	ann.Describe(&a.Private, "Whether the repository is private. Defaults to false (public).")
}

func (s *RepositoryState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.RepoID, "The canonical repository identifier: 'namespace/name'.")
	ann.Describe(&s.URL, "The web URL of the repository.")
}

// Ensure Repository implements all CRUD interfaces.
var _ infer.CustomResource[RepositoryArgs, RepositoryState] = (*Repository)(nil)

func resolveRepoType(t *string) string {
	if t == nil || *t == "" {
		return "model"
	}
	return *t
}

func (r *Repository) Create(
	ctx context.Context,
	req infer.CreateRequest[RepositoryArgs],
) (infer.CreateResponse[RepositoryState], error) {
	args := req.Inputs
	state := RepositoryState{RepositoryArgs: args}
	if req.DryRun {
		return infer.CreateResponse[RepositoryState]{ID: req.Name, Output: state}, nil
	}

	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.CreateResponse[RepositoryState]{}, fmt.Errorf("huggingface: API token is required")
	}
	client := newHFClient(token)

	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[RepositoryState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	repoType := resolveRepoType(args.RepoType)
	isPrivate := false
	if args.Private != nil {
		isPrivate = *args.Private
	}

	createReq := CreateRepoRequest{
		Name:    req.Name,
		Type:    repoType,
		Private: &isPrivate,
	}
	if namespace != "" {
		createReq.Organization = &namespace
	}

	repo, err := client.CreateRepo(createReq)
	if err != nil {
		return infer.CreateResponse[RepositoryState]{}, fmt.Errorf("create repository %q: %w", req.Name, err)
	}

	id := repo.ID
	if id == "" {
		id = fmt.Sprintf("%s/%s", namespace, req.Name)
	}
	ns := namespace
	state.Namespace = &ns
	state.RepoID = id
	rt := repoType
	state.RepoType = &rt
	state.URL = repoURL(repoType, id)
	return infer.CreateResponse[RepositoryState]{ID: id, Output: state}, nil
}

func (r *Repository) Read(
	ctx context.Context,
	req infer.ReadRequest[RepositoryArgs, RepositoryState],
) (infer.ReadResponse[RepositoryArgs, RepositoryState], error) {
	args := req.Inputs
	state := req.State
	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(args.Token, cfg.Token)
	if token == "" {
		return infer.ReadResponse[RepositoryArgs, RepositoryState]{ID: req.ID, Inputs: args, State: state}, nil
	}
	client := newHFClient(token)

	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.ReadResponse[RepositoryArgs, RepositoryState]{}, err
	}
	repoType := resolveRepoType(args.RepoType)
	_ = namespace

	repo, err := client.GetRepo(repoType, namespace, name)
	if err != nil {
		return infer.ReadResponse[RepositoryArgs, RepositoryState]{}, fmt.Errorf("read repository %q: %w", req.ID, err)
	}
	if repo == nil {
		return infer.ReadResponse[RepositoryArgs, RepositoryState]{}, nil
	}

	private := repo.Private
	state.Private = &private
	state.RepoID = req.ID
	state.URL = repoURL(repoType, req.ID)
	return infer.ReadResponse[RepositoryArgs, RepositoryState]{ID: req.ID, Inputs: args, State: state}, nil
}

func (r *Repository) Update(
	ctx context.Context,
	req infer.UpdateRequest[RepositoryArgs, RepositoryState],
) (infer.UpdateResponse[RepositoryState], error) {
	state := RepositoryState{RepositoryArgs: req.Inputs}
	state.RepoID = req.State.RepoID
	state.URL = req.State.URL
	// Repository visibility and metadata changes are not supported via the Hub API
	// in an atomic update call. Return the existing state; the primary mutable
	// property (files) is managed outside Pulumi.
	return infer.UpdateResponse[RepositoryState]{Output: state}, nil
}

func (r *Repository) Delete(
	ctx context.Context,
	req infer.DeleteRequest[RepositoryState],
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
	repoType := resolveRepoType(state.RepoType)
	if err := client.DeleteRepo(repoType, namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete repository %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

func repoURL(repoType, id string) string {
	switch repoType {
	case "dataset":
		return fmt.Sprintf("https://huggingface.co/datasets/%s", id)
	default:
		return fmt.Sprintf("https://huggingface.co/%s", id)
	}
}
