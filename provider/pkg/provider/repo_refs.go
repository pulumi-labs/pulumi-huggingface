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
// Shared helpers for git refs (branches and tags)
// ---------------------------------------------------------------------------

func checkRefRepo(repoID string, repoType *string) []p.CheckFailure {
	var failures []p.CheckFailure
	if n := strings.Count(repoID, "/"); n != 1 || strings.HasPrefix(repoID, "/") || strings.HasSuffix(repoID, "/") {
		failures = append(failures, p.CheckFailure{Property: "repoId", Reason: "repoId must be 'namespace/name'"})
	}
	if repoType != nil {
		switch *repoType {
		case "", "model", "dataset", "space":
		default:
			failures = append(failures, p.CheckFailure{Property: "repoType",
				Reason: fmt.Sprintf("invalid repoType %q; must be 'model', 'dataset' or 'space'", *repoType)})
		}
	}
	return failures
}

// createOnlyChanged reports a change in a create-only input. It cannot be
// read back, so an unset side (after an import, or when removed from the
// program) is never a change.
func createOnlyChanged(old, new *string) bool {
	return old != nil && new != nil && *old != *new
}

func findRef(refs []GitRef, name string) *GitRef {
	for i := range refs {
		if refs[i].Name == name {
			return &refs[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// RepositoryBranch resource
// ---------------------------------------------------------------------------

// RepositoryBranch is a git branch of a Hub repository.
type RepositoryBranch struct{}

// RepositoryBranchArgs are the inputs of a RepositoryBranch. Branches have
// nothing mutable: every property except the token replaces the branch.
type RepositoryBranchArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// RepoID is the repository, 'namespace/name'.
	RepoID string `pulumi:"repoId" provider:"replaceOnChanges"`
	// RepoType is 'model' (default), 'dataset' or 'space'.
	RepoType *string `pulumi:"repoType,optional" provider:"replaceOnChanges"`
	// Branch is the branch name.
	Branch string `pulumi:"branch" provider:"replaceOnChanges"`
	// StartingPoint is the revision (branch, tag or commit) the branch starts from.
	StartingPoint *string `pulumi:"startingPoint,optional"`
}

// RepositoryBranchState is the stored state of a RepositoryBranch.
type RepositoryBranchState struct {
	RepositoryBranchArgs
	// TargetCommit is the commit the branch points at when last read.
	TargetCommit string `pulumi:"targetCommit"`
}

func (r *RepositoryBranch) Annotate(a infer.Annotator) {
	a.Describe(&r, "A git branch of a Hugging Face Hub repository. The resource ID is '<models|datasets|spaces>/<namespace>/<name>@<branch>' "+
		"(for example: pulumi import huggingface:index:RepositoryBranch b models/acme/m@dev; without a prefix the repository is a model). "+
		"A branch has no mutable property, so any change replaces it.")
}

func (a *RepositoryBranchArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.RepoID, "The repository, 'namespace/name'. Changing it replaces the branch.")
	ann.Describe(&a.RepoType, "The repository type: 'model' (default), 'dataset' or 'space'. Changing it replaces the branch.")
	ann.Describe(&a.Branch, "The branch name. Changing it replaces the branch.")
	ann.Describe(&a.StartingPoint, "The revision (branch, tag or commit SHA) the branch is created from; defaults to the default branch head. "+
		"Create-only and not readable: after an import it is unset, and changing it replaces the branch.")
}

func (s *RepositoryBranchState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.TargetCommit, "The commit the branch pointed at when last read.")
}

var (
	_ infer.CustomResource[RepositoryBranchArgs, RepositoryBranchState] = (*RepositoryBranch)(nil)
	_ infer.CustomCheck[RepositoryBranchArgs]                           = (*RepositoryBranch)(nil)
	_ infer.CustomDiff[RepositoryBranchArgs, RepositoryBranchState]     = (*RepositoryBranch)(nil)
	_ infer.CustomRead[RepositoryBranchArgs, RepositoryBranchState]     = (*RepositoryBranch)(nil)
	_ infer.CustomUpdate[RepositoryBranchArgs, RepositoryBranchState]   = (*RepositoryBranch)(nil)
	_ infer.CustomDelete[RepositoryBranchState]                         = (*RepositoryBranch)(nil)
)

func (r *RepositoryBranch) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[RepositoryBranchArgs], error) {
	args, failures, err := infer.DefaultCheck[RepositoryBranchArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[RepositoryBranchArgs]{Inputs: args, Failures: failures}, err
	}
	failures = append(failures, checkRefRepo(args.RepoID, args.RepoType)...)
	if args.Branch == "" {
		failures = append(failures, p.CheckFailure{Property: "branch", Reason: "branch must not be empty"})
	}
	return infer.CheckResponse[RepositoryBranchArgs]{Inputs: args, Failures: dropUnknown(req.NewInputs, failures)}, nil
}

func refID(repoType, repoID, ref string) string { return refPrefix(repoType) + repoID + "@" + ref }

func (r *RepositoryBranch) Create(ctx context.Context, req infer.CreateRequest[RepositoryBranchArgs]) (infer.CreateResponse[RepositoryBranchState], error) {
	args := req.Inputs
	rt := resolveRepoType(args.RepoType)
	id := refID(rt, args.RepoID, args.Branch)
	state := RepositoryBranchState{RepositoryBranchArgs: args}
	if req.DryRun {
		return infer.CreateResponse[RepositoryBranchState]{ID: id, Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[RepositoryBranchState]{}, err
	}
	if err := client.CreateBranch(rt, args.RepoID, args.Branch, args.StartingPoint); err != nil {
		return infer.CreateResponse[RepositoryBranchState]{}, fmt.Errorf("create branch %q of %q: %w", args.Branch, args.RepoID, err)
	}
	refs, err := client.GetRefs(rt, args.RepoID)
	if err != nil {
		return infer.CreateResponse[RepositoryBranchState]{ID: id, Output: state},
			infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
	}
	if refs != nil {
		if b := findRef(refs.Branches, args.Branch); b != nil {
			state.TargetCommit = b.TargetCommit
		}
	}
	return infer.CreateResponse[RepositoryBranchState]{ID: id, Output: state}, nil
}

func (r *RepositoryBranch) Read(ctx context.Context, req infer.ReadRequest[RepositoryBranchArgs, RepositoryBranchState]) (infer.ReadResponse[RepositoryBranchArgs, RepositoryBranchState], error) {
	type resp = infer.ReadResponse[RepositoryBranchArgs, RepositoryBranchState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	rt, repoID, branch, err := splitRefID(req.ID)
	if err != nil {
		return resp{}, err
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	refs, err := client.GetRefs(rt, repoID)
	if err != nil {
		return resp{}, fmt.Errorf("read branch %q: %w", req.ID, err)
	}
	if refs == nil {
		return resp{}, nil
	}
	b := findRef(refs.Branches, branch)
	if b == nil {
		return resp{}, nil
	}
	imported := prior.RepoID == ""
	inputs := RepositoryBranchArgs{Token: prior.Token, RepoID: repoID, RepoType: prior.RepoType, Branch: branch, StartingPoint: firstNonNil(prior.StartingPoint, req.State.StartingPoint)}
	if imported {
		inputs.RepoType = ptr(rt)
	}
	stateArgs := inputs
	stateArgs.RepoType = ptr(rt)
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  RepositoryBranchState{RepositoryBranchArgs: stateArgs, TargetCommit: b.TargetCommit},
	}, nil
}

func (r *RepositoryBranch) Diff(_ context.Context, req infer.DiffRequest[RepositoryBranchArgs, RepositoryBranchState]) (infer.DiffResponse, error) {
	o, n := req.State.RepositoryBranchArgs, req.Inputs
	d := newDiffSet()
	idType, idRepo, idBranch, _ := splitRefID(req.ID)
	d.replace("repoId", idRepo != n.RepoID)
	d.replace("repoType", idType != resolveRepoType(n.RepoType))
	d.replace("branch", idBranch != n.Branch)
	d.replace("startingPoint", createOnlyChanged(o.StartingPoint, n.StartingPoint))
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	return d.response(), nil
}

// Update only records a new token or starting point input: the branch itself
// has nothing to change.
func (r *RepositoryBranch) Update(_ context.Context, req infer.UpdateRequest[RepositoryBranchArgs, RepositoryBranchState]) (infer.UpdateResponse[RepositoryBranchState], error) {
	return infer.UpdateResponse[RepositoryBranchState]{Output: RepositoryBranchState{RepositoryBranchArgs: req.Inputs, TargetCommit: req.State.TargetCommit}}, nil
}

func (r *RepositoryBranch) Delete(ctx context.Context, req infer.DeleteRequest[RepositoryBranchState]) (infer.DeleteResponse, error) {
	rt, repoID, branch, err := splitRefID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteBranch(rt, repoID, branch); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete branch %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// RepositoryTag resource
// ---------------------------------------------------------------------------

// RepositoryTag is a git tag of a Hub repository.
type RepositoryTag struct{}

// defaultRevision is where a tag is created when no revision is given.
const defaultRevision = "main"

// RepositoryTagArgs are the inputs of a RepositoryTag. Tags have nothing
// mutable: every property except the token replaces the tag.
type RepositoryTagArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// RepoID is the repository, 'namespace/name'.
	RepoID string `pulumi:"repoId" provider:"replaceOnChanges"`
	// RepoType is 'model' (default), 'dataset' or 'space'.
	RepoType *string `pulumi:"repoType,optional" provider:"replaceOnChanges"`
	// Tag is the tag name.
	Tag string `pulumi:"tag" provider:"replaceOnChanges"`
	// Revision is the branch, tag or commit SHA the tag is placed on.
	Revision *string `pulumi:"revision,optional"`
	// Message is the tag message.
	Message *string `pulumi:"message,optional"`
}

// RepositoryTagState is the stored state of a RepositoryTag.
type RepositoryTagState struct {
	RepositoryTagArgs
	// TargetCommit is the commit the tag points at when last read.
	TargetCommit string `pulumi:"targetCommit"`
}

func (r *RepositoryTag) Annotate(a infer.Annotator) {
	a.Describe(&r, "A git tag of a Hugging Face Hub repository. The resource ID is '<models|datasets|spaces>/<namespace>/<name>@<tag>' "+
		"(for example: pulumi import huggingface:index:RepositoryTag t models/acme/m@v1.0; without a prefix the repository is a model). "+
		"A tag has no mutable property, so any change replaces it.")
}

func (a *RepositoryTagArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.RepoID, "The repository, 'namespace/name'. Changing it replaces the tag.")
	ann.Describe(&a.RepoType, "The repository type: 'model' (default), 'dataset' or 'space'. Changing it replaces the tag.")
	ann.Describe(&a.Tag, "The tag name. Changing it replaces the tag.")
	ann.Describe(&a.Revision, "The branch, tag or commit SHA to tag; defaults to 'main'. Create-only and not readable: after an import it is unset, and changing it replaces the tag.")
	ann.Describe(&a.Message, "The tag message. Create-only and not readable; changing it replaces the tag.")
}

func (s *RepositoryTagState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.TargetCommit, "The commit the tag pointed at when last read.")
}

var (
	_ infer.CustomResource[RepositoryTagArgs, RepositoryTagState] = (*RepositoryTag)(nil)
	_ infer.CustomCheck[RepositoryTagArgs]                        = (*RepositoryTag)(nil)
	_ infer.CustomDiff[RepositoryTagArgs, RepositoryTagState]     = (*RepositoryTag)(nil)
	_ infer.CustomRead[RepositoryTagArgs, RepositoryTagState]     = (*RepositoryTag)(nil)
	_ infer.CustomUpdate[RepositoryTagArgs, RepositoryTagState]   = (*RepositoryTag)(nil)
	_ infer.CustomDelete[RepositoryTagState]                      = (*RepositoryTag)(nil)
)

func (r *RepositoryTag) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[RepositoryTagArgs], error) {
	args, failures, err := infer.DefaultCheck[RepositoryTagArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[RepositoryTagArgs]{Inputs: args, Failures: failures}, err
	}
	failures = append(failures, checkRefRepo(args.RepoID, args.RepoType)...)
	if args.Tag == "" {
		failures = append(failures, p.CheckFailure{Property: "tag", Reason: "tag must not be empty"})
	}
	return infer.CheckResponse[RepositoryTagArgs]{Inputs: args, Failures: dropUnknown(req.NewInputs, failures)}, nil
}

func (r *RepositoryTag) Create(ctx context.Context, req infer.CreateRequest[RepositoryTagArgs]) (infer.CreateResponse[RepositoryTagState], error) {
	args := req.Inputs
	rt := resolveRepoType(args.RepoType)
	id := refID(rt, args.RepoID, args.Tag)
	state := RepositoryTagState{RepositoryTagArgs: args}
	if req.DryRun {
		return infer.CreateResponse[RepositoryTagState]{ID: id, Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[RepositoryTagState]{}, err
	}
	if err := client.CreateTag(rt, args.RepoID, args.Tag, orDefault(args.Revision, defaultRevision), args.Message); err != nil {
		return infer.CreateResponse[RepositoryTagState]{}, fmt.Errorf("create tag %q of %q: %w", args.Tag, args.RepoID, err)
	}
	refs, err := client.GetRefs(rt, args.RepoID)
	if err != nil {
		return infer.CreateResponse[RepositoryTagState]{ID: id, Output: state},
			infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
	}
	if refs != nil {
		if t := findRef(refs.Tags, args.Tag); t != nil {
			state.TargetCommit = t.TargetCommit
		}
	}
	return infer.CreateResponse[RepositoryTagState]{ID: id, Output: state}, nil
}

func (r *RepositoryTag) Read(ctx context.Context, req infer.ReadRequest[RepositoryTagArgs, RepositoryTagState]) (infer.ReadResponse[RepositoryTagArgs, RepositoryTagState], error) {
	type resp = infer.ReadResponse[RepositoryTagArgs, RepositoryTagState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	rt, repoID, tag, err := splitRefID(req.ID)
	if err != nil {
		return resp{}, err
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	refs, err := client.GetRefs(rt, repoID)
	if err != nil {
		return resp{}, fmt.Errorf("read tag %q: %w", req.ID, err)
	}
	if refs == nil {
		return resp{}, nil
	}
	t := findRef(refs.Tags, tag)
	if t == nil {
		return resp{}, nil
	}
	imported := prior.RepoID == ""
	inputs := RepositoryTagArgs{
		Token: prior.Token, RepoID: repoID, RepoType: prior.RepoType, Tag: tag,
		Revision: firstNonNil(prior.Revision, req.State.Revision), Message: firstNonNil(prior.Message, req.State.Message),
	}
	if imported {
		inputs.RepoType = ptr(rt)
	}
	stateArgs := inputs
	stateArgs.RepoType = ptr(rt)
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  RepositoryTagState{RepositoryTagArgs: stateArgs, TargetCommit: t.TargetCommit},
	}, nil
}

func (r *RepositoryTag) Diff(_ context.Context, req infer.DiffRequest[RepositoryTagArgs, RepositoryTagState]) (infer.DiffResponse, error) {
	o, n := req.State.RepositoryTagArgs, req.Inputs
	d := newDiffSet()
	idType, idRepo, idTag, _ := splitRefID(req.ID)
	d.replace("repoId", idRepo != n.RepoID)
	d.replace("repoType", idType != resolveRepoType(n.RepoType))
	d.replace("tag", idTag != n.Tag)
	d.replace("revision", createOnlyChanged(o.Revision, n.Revision))
	d.replace("message", createOnlyChanged(o.Message, n.Message))
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	return d.response(), nil
}

// Update only records a new token: the tag itself has nothing to change.
func (r *RepositoryTag) Update(_ context.Context, req infer.UpdateRequest[RepositoryTagArgs, RepositoryTagState]) (infer.UpdateResponse[RepositoryTagState], error) {
	return infer.UpdateResponse[RepositoryTagState]{Output: RepositoryTagState{RepositoryTagArgs: req.Inputs, TargetCommit: req.State.TargetCommit}}, nil
}

func (r *RepositoryTag) Delete(ctx context.Context, req infer.DeleteRequest[RepositoryTagState]) (infer.DeleteResponse, error) {
	rt, repoID, tag, err := splitRefID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteTag(rt, repoID, tag); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete tag %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getRepositoryBranch / getRepositoryTag
// ---------------------------------------------------------------------------

// GetRepositoryBranch looks up a branch of a repository.
type GetRepositoryBranch struct{}

// GetRepositoryBranchArgs selects the branch.
type GetRepositoryBranchArgs struct {
	Token    *string `pulumi:"token,optional" provider:"secret"`
	RepoID   string  `pulumi:"repoId"`
	RepoType *string `pulumi:"repoType,optional"`
	Branch   string  `pulumi:"branch"`
}

// GetRepositoryBranchResult is the looked-up branch.
type GetRepositoryBranchResult struct {
	RepoID       string `pulumi:"repoId"`
	RepoType     string `pulumi:"repoType"`
	Branch       string `pulumi:"branch"`
	Ref          string `pulumi:"ref"`
	TargetCommit string `pulumi:"targetCommit"`
}

func (f *GetRepositoryBranch) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a branch of a Hugging Face Hub repository through its refs.")
}

func (a *GetRepositoryBranchArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.RepoID, "The repository, 'namespace/name'.")
	ann.Describe(&a.RepoType, "The repository type: 'model' (default), 'dataset' or 'space'.")
	ann.Describe(&a.Branch, "The branch name.")
}

func (r *GetRepositoryBranchResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.RepoID, "The repository.")
	ann.Describe(&r.RepoType, "The repository type.")
	ann.Describe(&r.Branch, "The branch name.")
	ann.Describe(&r.Ref, "The full git ref, for example 'refs/heads/main'.")
	ann.Describe(&r.TargetCommit, "The commit the branch points at.")
}

func (f *GetRepositoryBranch) Invoke(ctx context.Context, req infer.FunctionRequest[GetRepositoryBranchArgs]) (infer.FunctionResponse[GetRepositoryBranchResult], error) {
	out := infer.FunctionResponse[GetRepositoryBranchResult]{}
	in := req.Input
	if failures := checkRefRepo(in.RepoID, in.RepoType); len(failures) > 0 || in.Branch == "" {
		return out, fmt.Errorf("huggingface: getRepositoryBranch needs repoId ('namespace/name'), a valid repoType and a branch")
	}
	client, _, err := clientFor(ctx, in.Token)
	if err != nil {
		return out, err
	}
	rt := resolveRepoType(in.RepoType)
	refs, err := client.GetRefs(rt, in.RepoID)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up branch %q of %q: %w", in.Branch, in.RepoID, err)
	}
	if refs == nil {
		return out, fmt.Errorf("huggingface: repository %q was not found", in.RepoID)
	}
	b := findRef(refs.Branches, in.Branch)
	if b == nil {
		return out, fmt.Errorf("huggingface: branch %q was not found in %q", in.Branch, in.RepoID)
	}
	out.Output = GetRepositoryBranchResult{RepoID: in.RepoID, RepoType: rt, Branch: b.Name, Ref: b.Ref, TargetCommit: b.TargetCommit}
	return out, nil
}

// GetRepositoryTag looks up a tag of a repository.
type GetRepositoryTag struct{}

// GetRepositoryTagArgs selects the tag.
type GetRepositoryTagArgs struct {
	Token    *string `pulumi:"token,optional" provider:"secret"`
	RepoID   string  `pulumi:"repoId"`
	RepoType *string `pulumi:"repoType,optional"`
	Tag      string  `pulumi:"tag"`
}

// GetRepositoryTagResult is the looked-up tag.
type GetRepositoryTagResult struct {
	RepoID       string `pulumi:"repoId"`
	RepoType     string `pulumi:"repoType"`
	Tag          string `pulumi:"tag"`
	Ref          string `pulumi:"ref"`
	TargetCommit string `pulumi:"targetCommit"`
}

func (f *GetRepositoryTag) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a tag of a Hugging Face Hub repository through its refs.")
}

func (a *GetRepositoryTagArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.RepoID, "The repository, 'namespace/name'.")
	ann.Describe(&a.RepoType, "The repository type: 'model' (default), 'dataset' or 'space'.")
	ann.Describe(&a.Tag, "The tag name.")
}

func (r *GetRepositoryTagResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.RepoID, "The repository.")
	ann.Describe(&r.RepoType, "The repository type.")
	ann.Describe(&r.Tag, "The tag name.")
	ann.Describe(&r.Ref, "The full git ref, for example 'refs/tags/v1.0'.")
	ann.Describe(&r.TargetCommit, "The commit the tag points at.")
}

func (f *GetRepositoryTag) Invoke(ctx context.Context, req infer.FunctionRequest[GetRepositoryTagArgs]) (infer.FunctionResponse[GetRepositoryTagResult], error) {
	out := infer.FunctionResponse[GetRepositoryTagResult]{}
	in := req.Input
	if failures := checkRefRepo(in.RepoID, in.RepoType); len(failures) > 0 || in.Tag == "" {
		return out, fmt.Errorf("huggingface: getRepositoryTag needs repoId ('namespace/name'), a valid repoType and a tag")
	}
	client, _, err := clientFor(ctx, in.Token)
	if err != nil {
		return out, err
	}
	rt := resolveRepoType(in.RepoType)
	refs, err := client.GetRefs(rt, in.RepoID)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up tag %q of %q: %w", in.Tag, in.RepoID, err)
	}
	if refs == nil {
		return out, fmt.Errorf("huggingface: repository %q was not found", in.RepoID)
	}
	t := findRef(refs.Tags, in.Tag)
	if t == nil {
		return out, fmt.Errorf("huggingface: tag %q was not found in %q", in.Tag, in.RepoID)
	}
	out.Output = GetRepositoryTagResult{RepoID: in.RepoID, RepoType: rt, Tag: t.Name, Ref: t.Ref, TargetCommit: t.TargetCommit}
	return out, nil
}
