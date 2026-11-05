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
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
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
	// Defaults to the user associated with the API token when not set. Changing
	// it replaces the repository.
	Namespace *string `pulumi:"namespace,optional" provider:"replaceOnChanges"`

	// RepoType is the repository type: "model" (default) or "dataset". Changing
	// it replaces the repository.
	RepoType *string `pulumi:"repoType,optional" provider:"replaceOnChanges"`

	// Private controls whether the repository is private. Defaults to false.
	Private *bool `pulumi:"private,optional"`

	// Gated controls gated access: "off" (default), "auto" (access requests
	// are approved automatically) or "manual" (a maintainer approves each
	// request).
	Gated *string `pulumi:"gated,optional"`
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
		"container and visibility — not the files within it. Import with the ID 'namespace/name' "+
		"for a model or 'datasets/namespace/name' for a dataset.")
}

func (a *RepositoryArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.RepoType, "The repository type: 'model' (default) or 'dataset'. Changing it replaces the repository.")
	ann.Describe(&a.Private, "Whether the repository is private. Defaults to false (public).")
	ann.Describe(&a.Gated, "Gated access: 'off' (default), 'auto' (requests approved automatically) or 'manual' (a maintainer approves each request).")
	ann.Describe(&a.Namespace, "The owning user or organization. Defaults to the token's user. Changing it replaces the repository.")
}

func (s *RepositoryState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.RepoID, "The canonical repository identifier: 'namespace/name'.")
	ann.Describe(&s.URL, "The web URL of the repository.")
}

// Ensure Repository implements all CRUD interfaces.
var (
	_ infer.CustomResource[RepositoryArgs, RepositoryState] = (*Repository)(nil)
	_ infer.CustomCheck[RepositoryArgs]                     = (*Repository)(nil)
	_ infer.CustomDiff[RepositoryArgs, RepositoryState]     = (*Repository)(nil)
	_ infer.CustomRead[RepositoryArgs, RepositoryState]     = (*Repository)(nil)
	_ infer.CustomUpdate[RepositoryArgs, RepositoryState]   = (*Repository)(nil)
	_ infer.CustomDelete[RepositoryState]                   = (*Repository)(nil)
)

func resolveRepoType(t *string) string {
	if t == nil || *t == "" {
		return "model"
	}
	return *t
}

// repoResourceID builds the Pulumi resource ID. Models keep the plain
// "namespace/name" form; datasets carry the Hub's "datasets/" path prefix so
// the type can be recovered from the ID alone (needed for import).
func repoResourceID(repoType, id string) string {
	switch repoType {
	case "dataset":
		return "datasets/" + id
	case "space":
		return "spaces/" + id
	default:
		return id
	}
}

// parseRepoResourceID splits a Repository resource ID into its repo type,
// namespace and name. An ID without a type prefix is a model, unless hint
// (the type recorded in state by an earlier provider version, which gave
// datasets a plain "namespace/name" ID) says dataset.
func parseRepoResourceID(id string, hint *string) (repoType, namespace, name string, err error) {
	repoType = "model"
	if hint != nil && *hint == "dataset" {
		repoType = "dataset"
	}
	rest := id
	for prefix, t := range map[string]string{"datasets/": "dataset", "models/": "model", "spaces/": "space"} {
		if strings.HasPrefix(id, prefix) {
			repoType, rest = t, strings.TrimPrefix(id, prefix)
			break
		}
	}
	namespace, name, err = splitID(rest)
	return repoType, namespace, name, err
}

func (r *Repository) Check(
	ctx context.Context,
	req infer.CheckRequest,
) (infer.CheckResponse[RepositoryArgs], error) {
	args, failures, err := infer.DefaultCheck[RepositoryArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[RepositoryArgs]{Inputs: args, Failures: failures}, err
	}
	if args.RepoType != nil {
		switch *args.RepoType {
		case "", "model", "dataset":
		case "space":
			failures = append(failures, p.CheckFailure{
				Property: "repoType",
				Reason:   "use the Space resource to manage Spaces; repoType must be 'model' or 'dataset'",
			})
		default:
			failures = append(failures, p.CheckFailure{
				Property: "repoType",
				Reason:   fmt.Sprintf("invalid repoType %q; must be 'model' or 'dataset'", *args.RepoType),
			})
		}
	}
	if args.Gated != nil {
		switch *args.Gated {
		case "off", "auto", "manual":
		default:
			failures = append(failures, p.CheckFailure{
				Property: "gated",
				Reason:   fmt.Sprintf("invalid gated %q; must be 'off', 'auto' or 'manual'", *args.Gated),
			})
		}
	}
	return infer.CheckResponse[RepositoryArgs]{Inputs: args, Failures: failures}, nil
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

	client, token, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[RepositoryState]{}, err
	}
	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[RepositoryState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	repoType := resolveRepoType(args.RepoType)
	createReq := CreateRepoRequest{
		Name:         req.Name,
		Type:         repoType,
		Private:      ptr(orDefault(args.Private, false)),
		Organization: &namespace,
	}
	_, err = client.CreateRepo(createReq)
	if err != nil {
		return infer.CreateResponse[RepositoryState]{}, fmt.Errorf("create repository %q: %w", req.Name, err)
	}

	// The Hub's create response carries an internal ObjectId in id, not
	// namespace/name (#1549); the resource ID is always built from the names.
	id := fmt.Sprintf("%s/%s", namespace, req.Name)
	state.Namespace = ptr(namespace)
	state.RepoID = id
	state.RepoType = ptr(repoType)
	state.URL = repoURL(repoType, id)
	resID := repoResourceID(repoType, id)
	if g := orDefault(args.Gated, "off"); g != "off" {
		// Gating is a settings call after creation; the repository exists, so a
		// failure leaves it recorded with its partial state.
		if err := client.UpdateRepoSettings(repoType, namespace, req.Name, map[string]any{"gated": gatedSetting(g)}); err != nil {
			state.Gated = nil
			return infer.CreateResponse[RepositoryState]{ID: resID, Output: state},
				infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
		}
	}
	return infer.CreateResponse[RepositoryState]{ID: resID, Output: state}, nil
}

func (r *Repository) Read(
	ctx context.Context,
	req infer.ReadRequest[RepositoryArgs, RepositoryState],
) (infer.ReadResponse[RepositoryArgs, RepositoryState], error) {
	type resp = infer.ReadResponse[RepositoryArgs, RepositoryState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	repoType, namespace, name, err := parseRepoResourceID(req.ID, firstNonNil(req.State.RepoType, prior.RepoType))
	if err != nil {
		return resp{}, err
	}
	if repoType == "space" {
		return resp{}, fmt.Errorf("read repository %q: spaces are managed by the Space resource", req.ID)
	}

	repo, err := client.GetRepo(repoType, namespace, name)
	if err != nil {
		return resp{}, fmt.Errorf("read repository %q: %w", req.ID, err)
	}
	if repo == nil {
		// Deleted outside Pulumi: an empty ID drops the resource from state.
		return resp{}, nil
	}

	// An import has no prior inputs at all.
	imported := prior.Namespace == nil && prior.RepoType == nil && prior.Private == nil && req.State.RepoID == ""
	inputs := RepositoryArgs{
		Token:     prior.Token,
		Namespace: prior.Namespace,
		RepoType:  prior.RepoType,
	}
	if imported {
		inputs.Namespace = ptr(namespace)
		inputs.RepoType = ptr(repoType)
	}
	inputs.Private = refreshOpt(prior.Private, repo.Private, true, imported, ptr(false))
	apiGated := gatedValue(repo.Gated)
	inputs.Gated = refreshOpt(prior.Gated, apiGated, true, imported, ptr("off"))

	stateArgs := inputs
	stateArgs.Namespace = ptr(namespace)
	stateArgs.RepoType = ptr(repoType)
	stateArgs.Private = ptr(repo.Private)
	stateArgs.Gated = ptr(apiGated)
	repoID := namespace + "/" + name
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  RepositoryState{RepositoryArgs: stateArgs, RepoID: repoID, URL: repoURL(repoType, repoID)},
	}, nil
}

func (r *Repository) Diff(
	_ context.Context,
	req infer.DiffRequest[RepositoryArgs, RepositoryState],
) (infer.DiffResponse, error) {
	o, n := req.State.RepositoryArgs, req.Inputs
	d := newDiffSet()

	idType, idNS, _, _ := parseRepoResourceID(req.ID, req.State.RepoType)
	d.replace("namespace", namespaceChanged(idNS, n.Namespace))
	d.replace("repoType", idType != resolveRepoType(n.RepoType))

	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("private", orDefault(o.Private, false) != orDefault(n.Private, false))
	d.update("gated", orDefault(o.Gated, "off") != orDefault(n.Gated, "off"))
	return d.response(), nil
}

func (r *Repository) Update(
	ctx context.Context,
	req infer.UpdateRequest[RepositoryArgs, RepositoryState],
) (infer.UpdateResponse[RepositoryState], error) {
	state := RepositoryState{RepositoryArgs: req.Inputs, RepoID: req.State.RepoID, URL: req.State.URL}
	repoType, namespace, name, err := parseRepoResourceID(req.ID, req.State.RepoType)
	if err != nil {
		return infer.UpdateResponse[RepositoryState]{}, err
	}
	state.Namespace = ptr(namespace)
	state.RepoType = ptr(repoType)
	if req.DryRun {
		return infer.UpdateResponse[RepositoryState]{Output: state}, nil
	}

	client, _, err := clientFor(ctx, req.Inputs.Token)
	if err != nil {
		return infer.UpdateResponse[RepositoryState]{}, err
	}
	settings := map[string]any{}
	if orDefault(req.State.Private, false) != orDefault(req.Inputs.Private, false) {
		settings["private"] = orDefault(req.Inputs.Private, false)
	}
	if g := orDefault(req.Inputs.Gated, "off"); orDefault(req.State.Gated, "off") != g {
		settings["gated"] = gatedSetting(g)
	}
	if len(settings) > 0 {
		if err := client.UpdateRepoSettings(repoType, namespace, name, settings); err != nil {
			return infer.UpdateResponse[RepositoryState]{}, fmt.Errorf("update repository %q: %w", req.ID, err)
		}
	}
	return infer.UpdateResponse[RepositoryState]{Output: state}, nil
}

func (r *Repository) Delete(
	ctx context.Context,
	req infer.DeleteRequest[RepositoryState],
) (infer.DeleteResponse, error) {
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	repoType, namespace, name, err := parseRepoResourceID(req.ID, req.State.RepoType)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteRepo(repoType, namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete repository %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

func repoURL(repoType, id string) string {
	switch repoType {
	case "dataset":
		return fmt.Sprintf("https://huggingface.co/datasets/%s", id)
	case "space":
		return fmt.Sprintf("https://huggingface.co/spaces/%s", id)
	default:
		return fmt.Sprintf("https://huggingface.co/%s", id)
	}
}

func firstNonNil[T any](ps ...*T) *T {
	for _, p := range ps {
		if p != nil {
			return p
		}
	}
	return nil
}
