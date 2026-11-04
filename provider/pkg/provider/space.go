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

	p "github.com/pulumi/pulumi-go-provider"
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
	// Defaults to the user associated with the API token when not set. Changing
	// it replaces the Space.
	Namespace *string `pulumi:"namespace,optional" provider:"replaceOnChanges"`

	// SDK is the frontend framework: "gradio", "streamlit", "docker", or
	// "static". Changing it replaces the Space.
	SDK string `pulumi:"sdk" provider:"replaceOnChanges"`

	// Hardware is the compute tier for the Space. Defaults to "cpu-basic".
	// Examples: "cpu-basic", "cpu-upgrade", "t4-small", "a10g-small",
	// "a100-large". The Hub defines the full list of flavors.
	Hardware *string `pulumi:"hardware,optional"`

	// Private controls whether the Space is private. Defaults to false (public).
	Private *bool `pulumi:"private,optional"`

	// SleepTimeSeconds is the idle duration before hibernation. Only upgraded
	// hardware has a configurable sleep time. Left unmanaged when unset.
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
		"on Hugging Face infrastructure, backed by Gradio, Docker, or static HTML. "+
		"Import with the ID 'namespace/name'; secret values cannot be imported.")
}

func (a *SpaceArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.SDK, "The frontend framework: 'gradio', 'streamlit', 'docker', or 'static'. Changing it replaces the Space.")
	ann.Describe(&a.Hardware, "The compute tier (e.g. 'cpu-basic', 't4-small', 'a10g-small'). Defaults to 'cpu-basic'.")
	ann.Describe(&a.Secrets, "Secret environment variables injected at runtime. Values are write-only. "+
		"Keys removed from this list are deleted from the Space.")
	ann.Describe(&a.Variables, "Non-secret environment variables for the Space. Keys removed from this list are deleted from the Space.")
	ann.Describe(&a.SleepTimeSeconds, "Idle seconds before the Space sleeps (upgraded hardware only). Unmanaged when unset.")
}

func (s *SpaceState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.SpaceID, "The canonical Space identifier: 'namespace/name'.")
	ann.Describe(&s.URL, "The public URL of the Space.")
}

// Ensure Space implements all CRUD interfaces.
var (
	_ infer.CustomResource[SpaceArgs, SpaceState] = (*Space)(nil)
	_ infer.CustomCheck[SpaceArgs]                = (*Space)(nil)
	_ infer.CustomDiff[SpaceArgs, SpaceState]     = (*Space)(nil)
	_ infer.CustomRead[SpaceArgs, SpaceState]     = (*Space)(nil)
	_ infer.CustomUpdate[SpaceArgs, SpaceState]   = (*Space)(nil)
	_ infer.CustomDelete[SpaceState]              = (*Space)(nil)
)

const defaultSpaceHardware = string(SpaceHardwareCPUBasic)

func (r *Space) Check(
	ctx context.Context,
	req infer.CheckRequest,
) (infer.CheckResponse[SpaceArgs], error) {
	args, failures, err := infer.DefaultCheck[SpaceArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[SpaceArgs]{Inputs: args, Failures: failures}, err
	}
	switch SpaceSDK(args.SDK) {
	case SpaceSDKGradio, SpaceSDKStreamlit, SpaceSDKDocker, SpaceSDKStatic:
	default:
		failures = append(failures, p.CheckFailure{
			Property: "sdk",
			Reason:   fmt.Sprintf("invalid sdk %q; must be 'gradio', 'streamlit', 'docker' or 'static'", args.SDK),
		})
	}
	if args.Hardware != nil && (*args.Hardware == "h100" || *args.Hardware == "h100x8") {
		failures = append(failures, p.CheckFailure{
			Property: "hardware",
			Reason: fmt.Sprintf("%q is not a Space hardware flavor; use one of the Hub flavors such as "+
				"'a100-large', 'a100x4', 'a100x8' or 'l40sx1'", *args.Hardware),
		})
	}
	seen := map[string]bool{}
	for _, s := range args.Secrets {
		if seen[s.Key] {
			failures = append(failures, p.CheckFailure{Property: "secrets", Reason: fmt.Sprintf("duplicate secret key %q", s.Key)})
		}
		seen[s.Key] = true
	}
	seen = map[string]bool{}
	for _, v := range args.Variables {
		if seen[v.Key] {
			failures = append(failures, p.CheckFailure{Property: "variables", Reason: fmt.Sprintf("duplicate variable key %q", v.Key)})
		}
		seen[v.Key] = true
	}
	return infer.CheckResponse[SpaceArgs]{Inputs: args, Failures: failures}, nil
}

// kv is the comparable form of a secret or variable.
type kv struct{ value, description string }

func secretMap(in []SpaceSecretInput) map[string]kv {
	out := make(map[string]kv, len(in))
	for _, s := range in {
		out[s.Key] = kv{s.Value, orDefault(s.Description, "")}
	}
	return out
}

func variableMap(in []SpaceVariableInput) map[string]kv {
	out := make(map[string]kv, len(in))
	for _, v := range in {
		out[v.Key] = kv{v.Value, orDefault(v.Description, "")}
	}
	return out
}

func (r *Space) Create(
	ctx context.Context,
	req infer.CreateRequest[SpaceArgs],
) (infer.CreateResponse[SpaceState], error) {
	args := req.Inputs
	state := SpaceState{SpaceArgs: args}
	if req.DryRun {
		return infer.CreateResponse[SpaceState]{ID: req.Name, Output: state}, nil
	}

	client, token, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[SpaceState]{}, err
	}
	namespace, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[SpaceState]{}, fmt.Errorf("resolve namespace: %w", err)
	}

	var secrets []SpaceSecret
	for _, s := range args.Secrets {
		secrets = append(secrets, SpaceSecret{Key: s.Key, Value: s.Value, Description: orDefault(s.Description, "")})
	}
	var variables []SpaceEnvVar
	for _, v := range args.Variables {
		variables = append(variables, SpaceEnvVar{Key: v.Key, Value: v.Value, Description: orDefault(v.Description, "")})
	}

	createReq := CreateRepoRequest{
		Name:         req.Name,
		Type:         "space",
		Organization: &namespace,
		Private:      ptr(orDefault(args.Private, false)),
		SDK:          ptr(args.SDK),
		Hardware:     ptr(orDefault(args.Hardware, defaultSpaceHardware)),
		SleepTime:    args.SleepTimeSeconds,
		Secrets:      secrets,
		Variables:    variables,
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
	state.Namespace = ptr(namespace)
	return infer.CreateResponse[SpaceState]{ID: id, Output: state}, nil
}

func (r *Space) Read(
	ctx context.Context,
	req infer.ReadRequest[SpaceArgs, SpaceState],
) (infer.ReadResponse[SpaceArgs, SpaceState], error) {
	type resp = infer.ReadResponse[SpaceArgs, SpaceState]
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

	repo, err := client.GetRepo("space", namespace, name)
	if err != nil {
		return resp{}, fmt.Errorf("read space %q: %w", req.ID, err)
	}
	if repo == nil {
		// Deleted outside Pulumi: an empty ID drops the resource from state.
		return resp{}, nil
	}
	apiVars, err := client.GetSpaceVariables(namespace, name)
	if err != nil {
		return resp{}, fmt.Errorf("read space %q: %w", req.ID, err)
	}
	apiSecretKeys, err := client.GetSpaceSecretKeys(namespace, name)
	if err != nil {
		return resp{}, fmt.Errorf("read space %q: %w", req.ID, err)
	}

	imported := prior.SDK == ""
	inputs := SpaceArgs{
		Token:     prior.Token,
		Namespace: prior.Namespace,
		SDK:       prior.SDK,
	}
	if repo.SDK != nil && *repo.SDK != "" {
		inputs.SDK = *repo.SDK
	}
	if imported {
		inputs.Namespace = ptr(namespace)
	}
	hw, hasHW := repo.EffectiveHardware()
	inputs.Hardware = refreshOpt(prior.Hardware, hw, hasHW, imported, ptr(defaultSpaceHardware))
	inputs.Private = refreshOpt(prior.Private, repo.Private, true, imported, ptr(false))
	var gc int
	hasGC := repo.Runtime != nil && repo.Runtime.GCTimeout != nil
	if hasGC {
		gc = *repo.Runtime.GCTimeout
	}
	inputs.SleepTimeSeconds = refreshOpt(prior.SleepTimeSeconds, gc, hasGC, imported, nil)

	// Variables: an import adopts every variable; a refresh compares the keys
	// the user manages, keeping their order.
	if imported {
		for _, k := range slices.Sorted(maps.Keys(apiVars)) {
			v := apiVars[k]
			vi := SpaceVariableInput{Key: k, Value: v.Value}
			if v.Description != "" {
				vi.Description = ptr(v.Description)
			}
			inputs.Variables = append(inputs.Variables, vi)
		}
	} else {
		for _, pv := range prior.Variables {
			if av, ok := apiVars[pv.Key]; ok {
				vi := SpaceVariableInput{Key: pv.Key, Value: av.Value, Description: pv.Description}
				if av.Description != orDefault(pv.Description, "") {
					vi.Description = ptr(av.Description)
				}
				inputs.Variables = append(inputs.Variables, vi)
			}
		}
	}
	// Secrets: values are write-only, so only the presence of managed keys is
	// checked. They cannot be imported.
	if !imported {
		for _, ps := range prior.Secrets {
			if apiSecretKeys[ps.Key] {
				inputs.Secrets = append(inputs.Secrets, ps)
			}
		}
	}

	stateArgs := inputs
	stateArgs.Namespace = ptr(namespace)
	if hasHW {
		stateArgs.Hardware = ptr(hw)
	}
	stateArgs.Private = ptr(repo.Private)
	if hasGC {
		stateArgs.SleepTimeSeconds = ptr(gc)
	}
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State: SpaceState{
			SpaceArgs: stateArgs,
			SpaceID:   req.ID,
			URL:       fmt.Sprintf("https://huggingface.co/spaces/%s", req.ID),
		},
	}, nil
}

func (r *Space) Diff(
	_ context.Context,
	req infer.DiffRequest[SpaceArgs, SpaceState],
) (infer.DiffResponse, error) {
	o, n := req.State.SpaceArgs, req.Inputs
	d := newDiffSet()

	idNS, _, _ := splitID(req.ID)
	d.replace("namespace", namespaceChanged(idNS, n.Namespace))
	d.replace("sdk", o.SDK != n.SDK)

	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("hardware", orDefault(o.Hardware, defaultSpaceHardware) != orDefault(n.Hardware, defaultSpaceHardware))
	d.update("private", orDefault(o.Private, false) != orDefault(n.Private, false))
	d.update("sleepTimeSeconds", ptrChanged(o.SleepTimeSeconds, n.SleepTimeSeconds))
	d.update("secrets", !maps.Equal(secretMap(o.Secrets), secretMap(n.Secrets)))
	d.update("variables", !maps.Equal(variableMap(o.Variables), variableMap(n.Variables)))
	return d.response(), nil
}

func (r *Space) Update(
	ctx context.Context,
	req infer.UpdateRequest[SpaceArgs, SpaceState],
) (infer.UpdateResponse[SpaceState], error) {
	args, old := req.Inputs, req.State.SpaceArgs
	state := SpaceState{SpaceArgs: args, SpaceID: req.State.SpaceID, URL: req.State.URL}
	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.UpdateResponse[SpaceState]{}, err
	}
	state.Namespace = ptr(namespace)
	if req.DryRun {
		return infer.UpdateResponse[SpaceState]{Output: state}, nil
	}

	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.UpdateResponse[SpaceState]{}, err
	}
	fail := func(what string, err error) (infer.UpdateResponse[SpaceState], error) {
		return infer.UpdateResponse[SpaceState]{}, fmt.Errorf("update space %q %s: %w", req.ID, what, err)
	}

	if orDefault(old.Private, false) != orDefault(args.Private, false) {
		if err := client.UpdateRepoVisibility("space", namespace, name, orDefault(args.Private, false)); err != nil {
			return fail("visibility", err)
		}
	}
	if orDefault(old.Hardware, defaultSpaceHardware) != orDefault(args.Hardware, defaultSpaceHardware) {
		if err := client.UpdateSpaceHardware(namespace, name, SpaceHardware(orDefault(args.Hardware, defaultSpaceHardware))); err != nil {
			return fail("hardware", err)
		}
	}
	if args.SleepTimeSeconds != nil && ptrChanged(old.SleepTimeSeconds, args.SleepTimeSeconds) {
		if err := client.UpdateSpaceSleepTime(namespace, name, *args.SleepTimeSeconds); err != nil {
			return fail("sleep time", err)
		}
	}

	// Variables and secrets are synced key by key: upsert what is new or
	// changed, delete what was removed from the program.
	newVars, oldVars := variableMap(args.Variables), variableMap(old.Variables)
	for _, v := range args.Variables {
		if cur, ok := oldVars[v.Key]; ok && cur == newVars[v.Key] {
			continue
		}
		if err := client.UpsertSpaceVariable(namespace, name, SpaceEnvVar{Key: v.Key, Value: v.Value, Description: orDefault(v.Description, "")}); err != nil {
			return fail("variables", err)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(oldVars)) {
		if _, ok := newVars[k]; !ok {
			if err := client.DeleteSpaceVariable(namespace, name, k); err != nil {
				return fail("variables", err)
			}
		}
	}
	newSecrets, oldSecrets := secretMap(args.Secrets), secretMap(old.Secrets)
	for _, s := range args.Secrets {
		if cur, ok := oldSecrets[s.Key]; ok && cur == newSecrets[s.Key] {
			continue
		}
		if err := client.UpsertSpaceSecret(namespace, name, SpaceSecret{Key: s.Key, Value: s.Value, Description: orDefault(s.Description, "")}); err != nil {
			return fail("secrets", err)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(oldSecrets)) {
		if _, ok := newSecrets[k]; !ok {
			if err := client.DeleteSpaceSecret(namespace, name, k); err != nil {
				return fail("secrets", err)
			}
		}
	}

	return infer.UpdateResponse[SpaceState]{Output: state}, nil
}

func (r *Space) Delete(
	ctx context.Context,
	req infer.DeleteRequest[SpaceState],
) (infer.DeleteResponse, error) {
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	namespace, name, err := splitID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteRepo("space", namespace, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete space %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
