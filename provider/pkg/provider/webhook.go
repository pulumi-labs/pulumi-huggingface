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
	"slices"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Webhook resource
// ---------------------------------------------------------------------------

// Webhook is a Hub webhook (Settings > Webhooks) that posts events for
// watched users, organizations, models, datasets, Spaces or buckets to a URL.
// Webhooks that trigger a Job are not covered yet (see the coverage inventory).
type Webhook struct{}

// defaultWebhookDomains is what the Hub watches when no domains are given.
var defaultWebhookDomains = []string{"repo", "discussion"}

// WebhookWatchedItem is one thing a webhook watches.
type WebhookWatchedItem struct {
	// Type is one of bucket, dataset, model, org, space, user.
	Type string `pulumi:"type"`
	// Name is the item's name: a user or organization name, or a repo ID.
	Name string `pulumi:"name"`
}

func (w *WebhookWatchedItem) Annotate(a infer.Annotator) {
	a.Describe(&w.Type, "The kind of item to watch: 'bucket', 'dataset', 'model', 'org', 'space' or 'user'.")
	a.Describe(&w.Name, "The item's name: a user or organization name, or a repository ID such as 'namespace/name'.")
}

// WebhookArgs are the inputs of a Webhook.
type WebhookArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// URL receives the webhook payloads.
	URL string `pulumi:"url"`
	// Watched lists the items to watch.
	Watched []WebhookWatchedItem `pulumi:"watched"`
	// Domains selects the event domains: repo and/or discussion.
	Domains []string `pulumi:"domains,optional"`
	// Secret signs the payloads (write-only).
	Secret *string `pulumi:"secret,optional" provider:"secret"`
	// Disabled pauses the webhook without deleting it.
	Disabled *bool `pulumi:"disabled,optional"`
}

// WebhookState is the stored state of a Webhook.
type WebhookState struct {
	WebhookArgs
	// EffectiveDomains is what the Hub reports as watched domains.
	EffectiveDomains []string `pulumi:"effectiveDomains"`
}

func (r *Webhook) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Hub webhook that sends events for watched items to a URL. "+
		"The resource ID is the webhook ID; import with it (for example: pulumi import huggingface:index:Webhook w 64f9a55bb3115b4f513ec026). "+
		"The secret is write-only and cannot be imported: set it again after import. "+
		"Webhooks that trigger a Job are not supported yet.")
}

func (a *WebhookArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.URL, "The URL that receives the webhook payloads.")
	ann.Describe(&a.Watched, "The items the webhook watches.")
	ann.Describe(&a.Domains, "Event domains to watch: 'repo' and/or 'discussion'. When unset the Hub watches both and the value is not managed.")
	ann.Describe(&a.Secret, "Secret used to sign payloads. Write-only: the API never returns it, so it is kept from state and cannot be imported. Removing it from the program does not clear it on the Hub.")
	ann.Describe(&a.Disabled, "Whether the webhook is disabled. Defaults to false (enabled).")
}

func (s *WebhookState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.EffectiveDomains, "The domains the Hub reports as watched.")
}

var (
	_ infer.CustomResource[WebhookArgs, WebhookState] = (*Webhook)(nil)
	_ infer.CustomCheck[WebhookArgs]                  = (*Webhook)(nil)
	_ infer.CustomDiff[WebhookArgs, WebhookState]     = (*Webhook)(nil)
	_ infer.CustomRead[WebhookArgs, WebhookState]     = (*Webhook)(nil)
	_ infer.CustomUpdate[WebhookArgs, WebhookState]   = (*Webhook)(nil)
	_ infer.CustomDelete[WebhookState]                = (*Webhook)(nil)
)

var webhookWatchTypes = []string{"bucket", "dataset", "model", "org", "space", "user"}

func (r *Webhook) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[WebhookArgs], error) {
	args, failures, err := infer.DefaultCheck[WebhookArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[WebhookArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	if args.URL == "" {
		fail("url", "url must not be empty")
	}
	if len(args.Watched) == 0 {
		fail("watched", "watch at least one item")
	}
	for i, w := range args.Watched {
		if !slices.Contains(webhookWatchTypes, w.Type) {
			fail(fmt.Sprintf("watched[%d].type", i), fmt.Sprintf("invalid type %q; must be one of %v", w.Type, webhookWatchTypes))
		}
		if w.Name == "" {
			fail(fmt.Sprintf("watched[%d].name", i), "name must not be empty")
		}
	}
	for _, d := range args.Domains {
		if !slices.Contains(defaultWebhookDomains, d) {
			fail("domains", fmt.Sprintf("invalid domain %q; must be 'repo' or 'discussion'", d))
		}
	}
	return infer.CheckResponse[WebhookArgs]{Inputs: args, Failures: dropUnknown(req.NewInputs, failures)}, nil
}

func watchedToAPI(in []WebhookWatchedItem) []WebhookWatched {
	out := make([]WebhookWatched, 0, len(in))
	for _, w := range in {
		out = append(out, WebhookWatched{Type: w.Type, Name: w.Name})
	}
	return out
}

func watchedFromAPI(in []WebhookWatched) []WebhookWatchedItem {
	out := make([]WebhookWatchedItem, 0, len(in))
	for _, w := range in {
		out = append(out, WebhookWatchedItem{Type: w.Type, Name: w.Name})
	}
	return out
}

func sortedWatched(in []WebhookWatchedItem) []WebhookWatchedItem {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b WebhookWatchedItem) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		if a.Name < b.Name {
			return -1
		} else if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}

func sameWatched(a, b []WebhookWatchedItem) bool {
	return slices.Equal(sortedWatched(a), sortedWatched(b))
}

func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func (r *Webhook) Create(ctx context.Context, req infer.CreateRequest[WebhookArgs]) (infer.CreateResponse[WebhookState], error) {
	args := req.Inputs
	state := WebhookState{WebhookArgs: args, EffectiveDomains: orSlice(args.Domains, defaultWebhookDomains)}
	if req.DryRun {
		return infer.CreateResponse[WebhookState]{ID: req.Name, Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[WebhookState]{}, err
	}
	payload := map[string]any{
		"url":     args.URL,
		"watched": watchedToAPI(args.Watched),
		"domains": orSlice(args.Domains, defaultWebhookDomains),
	}
	if args.Secret != nil {
		payload["secret"] = *args.Secret
	}
	wh, err := client.CreateWebhook(payload)
	if err != nil {
		return infer.CreateResponse[WebhookState]{}, fmt.Errorf("create webhook: %w", err)
	}
	state.EffectiveDomains = wh.Domains
	if orDefault(args.Disabled, false) {
		if err := client.SetWebhookEnabled(wh.ID, false); err != nil {
			s := state
			s.Disabled = nil
			return infer.CreateResponse[WebhookState]{ID: wh.ID, Output: s},
				infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
		}
	}
	return infer.CreateResponse[WebhookState]{ID: wh.ID, Output: state}, nil
}

func orSlice(s, def []string) []string {
	if s != nil {
		return slices.Clone(s)
	}
	return slices.Clone(def)
}

func (r *Webhook) Read(ctx context.Context, req infer.ReadRequest[WebhookArgs, WebhookState]) (infer.ReadResponse[WebhookArgs, WebhookState], error) {
	type resp = infer.ReadResponse[WebhookArgs, WebhookState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	if prior.Secret == nil {
		prior.Secret = req.State.Secret
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	wh, err := client.GetWebhook(req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read webhook %q: %w", req.ID, err)
	}
	if wh == nil {
		return resp{}, nil
	}
	imported := prior.URL == ""
	apiWatched := watchedFromAPI(wh.Watched)
	inputs := WebhookArgs{Token: prior.Token, Secret: prior.Secret, URL: orDefault(wh.URL, ""), Watched: apiWatched}
	if !imported && sameWatched(prior.Watched, apiWatched) {
		inputs.Watched = prior.Watched
	}
	switch {
	case imported:
		inputs.Domains = slices.Clone(wh.Domains)
	case prior.Domains != nil:
		inputs.Domains = prior.Domains
		if !sameSet(prior.Domains, wh.Domains) {
			inputs.Domains = slices.Clone(wh.Domains)
		}
	case !sameSet(wh.Domains, defaultWebhookDomains):
		// Unset means the default; the API differs, so surface the drift.
		inputs.Domains = slices.Clone(wh.Domains)
	}
	inputs.Disabled = refreshOpt(prior.Disabled, wh.Disabled, true, imported, ptr(false))

	stateArgs := inputs
	stateArgs.Disabled = ptr(wh.Disabled)
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  WebhookState{WebhookArgs: stateArgs, EffectiveDomains: wh.Domains},
	}, nil
}

func (r *Webhook) Diff(_ context.Context, req infer.DiffRequest[WebhookArgs, WebhookState]) (infer.DiffResponse, error) {
	o, n := req.State.WebhookArgs, req.Inputs
	d := newDiffSet()
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("url", o.URL != n.URL)
	d.update("watched", !sameWatched(o.Watched, n.Watched))
	// Unset domains mean the default (both), so they are compared too.
	d.update("domains", !sameSet(orSlice(n.Domains, defaultWebhookDomains), orSlice(req.State.EffectiveDomains, defaultWebhookDomains)))
	d.update("secret", n.Secret != nil && orDefault(o.Secret, "\x00") != *n.Secret)
	d.update("disabled", orDefault(o.Disabled, false) != orDefault(n.Disabled, false))
	return d.response(), nil
}

func (r *Webhook) Update(ctx context.Context, req infer.UpdateRequest[WebhookArgs, WebhookState]) (infer.UpdateResponse[WebhookState], error) {
	args, old := req.Inputs, req.State.WebhookArgs
	state := WebhookState{WebhookArgs: args, EffectiveDomains: req.State.EffectiveDomains}
	if req.DryRun {
		state.EffectiveDomains = orSlice(args.Domains, defaultWebhookDomains)
		return infer.UpdateResponse[WebhookState]{Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.UpdateResponse[WebhookState]{}, err
	}
	// The update call always carries the full watched list.
	payload := map[string]any{"watched": watchedToAPI(args.Watched), "url": args.URL, "domains": orSlice(args.Domains, defaultWebhookDomains)}
	if args.Secret != nil && orDefault(old.Secret, "\x00") != *args.Secret {
		payload["secret"] = *args.Secret
	}
	wh, err := client.UpdateWebhook(req.ID, payload)
	if err != nil {
		return infer.UpdateResponse[WebhookState]{}, fmt.Errorf("update webhook %q: %w", req.ID, err)
	}
	state.EffectiveDomains = wh.Domains
	if orDefault(old.Disabled, false) != orDefault(args.Disabled, false) {
		if err := client.SetWebhookEnabled(req.ID, !orDefault(args.Disabled, false)); err != nil {
			s := state
			s.Disabled = old.Disabled
			return infer.UpdateResponse[WebhookState]{Output: s},
				infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
		}
	}
	return infer.UpdateResponse[WebhookState]{Output: state}, nil
}

func (r *Webhook) Delete(ctx context.Context, req infer.DeleteRequest[WebhookState]) (infer.DeleteResponse, error) {
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteWebhook(req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete webhook %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getWebhook
// ---------------------------------------------------------------------------

// GetWebhook looks up a webhook by ID.
type GetWebhook struct{}

// GetWebhookArgs selects the webhook.
type GetWebhookArgs struct {
	Token     *string `pulumi:"token,optional" provider:"secret"`
	WebhookID string  `pulumi:"webhookId"`
}

// GetWebhookResult is the looked-up webhook. The secret is write-only and
// not part of the result.
type GetWebhookResult struct {
	WebhookID string               `pulumi:"webhookId"`
	URL       string               `pulumi:"url"`
	Watched   []WebhookWatchedItem `pulumi:"watched"`
	Domains   []string             `pulumi:"domains"`
	Disabled  bool                 `pulumi:"disabled"`
}

func (f *GetWebhook) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Hub webhook by ID. The webhook secret is write-only and is not part of the result.")
}

func (a *GetWebhookArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.WebhookID, "The webhook ID.")
}

func (r *GetWebhookResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.WebhookID, "The webhook ID.")
	ann.Describe(&r.URL, "The URL that receives the payloads.")
	ann.Describe(&r.Watched, "The watched items.")
	ann.Describe(&r.Domains, "The watched event domains.")
	ann.Describe(&r.Disabled, "Whether the webhook is disabled.")
}

func (f *GetWebhook) Invoke(ctx context.Context, req infer.FunctionRequest[GetWebhookArgs]) (infer.FunctionResponse[GetWebhookResult], error) {
	out := infer.FunctionResponse[GetWebhookResult]{}
	if req.Input.WebhookID == "" {
		return out, fmt.Errorf("huggingface: getWebhook needs a webhookId")
	}
	client, _, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	wh, err := client.GetWebhook(req.Input.WebhookID)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up webhook %q: %w", req.Input.WebhookID, err)
	}
	if wh == nil {
		return out, fmt.Errorf("huggingface: webhook %q was not found", req.Input.WebhookID)
	}
	out.Output = GetWebhookResult{
		WebhookID: wh.ID, URL: orDefault(wh.URL, ""), Watched: watchedFromAPI(wh.Watched),
		Domains: wh.Domains, Disabled: wh.Disabled,
	}
	return out, nil
}
