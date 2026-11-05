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
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Collection resource
// ---------------------------------------------------------------------------

// Collection is a Hub collection: a curated, titled list of models, datasets,
// Spaces, papers, buckets and other collections. Its items are managed by
// CollectionItem resources.
type Collection struct{}

// CollectionArgs are the inputs of a Collection.
type CollectionArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// Title is the collection title.
	Title string `pulumi:"title"`
	// Namespace is the owning user or organization. Defaults to the token's user.
	Namespace *string `pulumi:"namespace,optional" provider:"replaceOnChanges"`
	// Description is a plain-text description.
	Description *string `pulumi:"description,optional"`
	// Private controls visibility. Defaults to false.
	Private *bool `pulumi:"private,optional"`
	// Theme is the collection's colour theme.
	Theme *string `pulumi:"theme,optional"`
}

// CollectionState is the stored state of a Collection.
type CollectionState struct {
	CollectionArgs
	// Slug is the collection slug, "namespace/title-slug-hash".
	Slug string `pulumi:"slug"`
	// URL is the web URL of the collection.
	URL string `pulumi:"url"`
	// EffectiveTheme is the theme the Hub reports (the Hub picks one when theme is unset).
	EffectiveTheme string `pulumi:"effectiveTheme"`
}

func (r *Collection) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Hub collection. The resource ID is the collection slug "+
		"('namespace/title-slug-hash'); import with it (for example: pulumi import huggingface:index:Collection c ns/recent-models-64f9a55bb3115b4f513ec026). "+
		"Items belong to CollectionItem resources.")
}

func (a *CollectionArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Title, "The collection title.")
	ann.Describe(&a.Namespace, "The owning user or organization. Defaults to the token's user. Changing it replaces the collection.")
	ann.Describe(&a.Description, "Plain-text description of the collection.")
	ann.Describe(&a.Private, "Whether the collection is private. Defaults to false.")
	ann.Describe(&a.Theme, "The collection's colour theme. Unmanaged while unset (the Hub picks one).")
}

func (s *CollectionState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.Slug, "The collection slug: 'namespace/title-slug-hash'.")
	ann.Describe(&s.URL, "The web URL of the collection.")
	ann.Describe(&s.EffectiveTheme, "The theme the Hub reports for the collection.")
}

var (
	_ infer.CustomResource[CollectionArgs, CollectionState] = (*Collection)(nil)
	_ infer.CustomCheck[CollectionArgs]                     = (*Collection)(nil)
	_ infer.CustomDiff[CollectionArgs, CollectionState]     = (*Collection)(nil)
	_ infer.CustomRead[CollectionArgs, CollectionState]     = (*Collection)(nil)
	_ infer.CustomUpdate[CollectionArgs, CollectionState]   = (*Collection)(nil)
	_ infer.CustomDelete[CollectionState]                   = (*Collection)(nil)
)

func (r *Collection) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[CollectionArgs], error) {
	args, failures, err := infer.DefaultCheck[CollectionArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[CollectionArgs]{Inputs: args, Failures: failures}, err
	}
	if strings.TrimSpace(args.Title) == "" {
		failures = append(failures, p.CheckFailure{Property: "title", Reason: "title must not be empty"})
	}
	return infer.CheckResponse[CollectionArgs]{Inputs: args, Failures: dropUnknown(req.NewInputs, failures)}, nil
}

func slugNamespace(slug string) string {
	ns, _, _ := strings.Cut(slug, "/")
	return ns
}

func collectionURL(slug string) string { return "https://huggingface.co/collections/" + slug }

func (r *Collection) Create(ctx context.Context, req infer.CreateRequest[CollectionArgs]) (infer.CreateResponse[CollectionState], error) {
	args := req.Inputs
	state := CollectionState{CollectionArgs: args}
	if req.DryRun {
		return infer.CreateResponse[CollectionState]{ID: req.Name, Output: state}, nil
	}
	client, token, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[CollectionState]{}, err
	}
	ns, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[CollectionState]{}, fmt.Errorf("resolve namespace: %w", err)
	}
	payload := map[string]any{"title": args.Title, "namespace": ns, "private": orDefault(args.Private, false)}
	if args.Description != nil {
		payload["description"] = *args.Description
	}
	col, err := client.CreateCollection(payload)
	if err != nil {
		return infer.CreateResponse[CollectionState]{}, fmt.Errorf("create collection %q: %w", args.Title, err)
	}
	state.Namespace = ptr(ns)
	state.Slug, state.URL, state.EffectiveTheme = col.Slug, collectionURL(col.Slug), col.Theme
	if args.Theme != nil && *args.Theme != col.Theme {
		// The create call has no theme; set it afterwards.
		if err := client.UpdateCollection(col.Slug, map[string]any{"theme": *args.Theme}); err != nil {
			s := state
			s.Theme = nil
			return infer.CreateResponse[CollectionState]{ID: col.Slug, Output: s},
				infer.ResourceInitFailedError{Reasons: []string{err.Error()}}
		}
		state.EffectiveTheme = *args.Theme
	}
	return infer.CreateResponse[CollectionState]{ID: col.Slug, Output: state}, nil
}

func (r *Collection) Read(ctx context.Context, req infer.ReadRequest[CollectionArgs, CollectionState]) (infer.ReadResponse[CollectionArgs, CollectionState], error) {
	type resp = infer.ReadResponse[CollectionArgs, CollectionState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	col, err := client.GetCollection(req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read collection %q: %w", req.ID, err)
	}
	if col == nil {
		return resp{}, nil
	}
	imported := prior.Title == ""
	inputs := CollectionArgs{Token: prior.Token, Title: col.Title, Namespace: prior.Namespace}
	if imported {
		inputs.Namespace = ptr(slugNamespace(req.ID))
	}
	inputs.Description = refreshOpt(prior.Description, col.Description, !imported || col.Description != "", imported, ptr(""))
	inputs.Private = refreshOpt(prior.Private, col.Private, true, imported, ptr(false))
	inputs.Theme = refreshOpt(prior.Theme, col.Theme, col.Theme != "", imported, nil)

	stateArgs := inputs
	stateArgs.Namespace = ptr(slugNamespace(req.ID))
	stateArgs.Private = ptr(col.Private)
	if col.Description != "" || prior.Description != nil {
		stateArgs.Description = ptr(col.Description)
	}
	if col.Theme != "" {
		stateArgs.Theme = ptr(col.Theme)
	}
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  CollectionState{CollectionArgs: stateArgs, Slug: col.Slug, URL: collectionURL(col.Slug), EffectiveTheme: col.Theme},
	}, nil
}

func (r *Collection) Diff(_ context.Context, req infer.DiffRequest[CollectionArgs, CollectionState]) (infer.DiffResponse, error) {
	o, n := req.State.CollectionArgs, req.Inputs
	d := newDiffSet()
	d.replace("namespace", namespaceChanged(slugNamespace(req.ID), n.Namespace))
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("title", o.Title != n.Title)
	d.update("description", ptrChanged(o.Description, n.Description))
	d.update("private", orDefault(o.Private, false) != orDefault(n.Private, false))
	d.update("theme", n.Theme != nil && *n.Theme != req.State.EffectiveTheme)
	return d.response(), nil
}

func (r *Collection) Update(ctx context.Context, req infer.UpdateRequest[CollectionArgs, CollectionState]) (infer.UpdateResponse[CollectionState], error) {
	args, old := req.Inputs, req.State
	state := CollectionState{CollectionArgs: args, Slug: old.Slug, URL: old.URL, EffectiveTheme: old.EffectiveTheme}
	state.Namespace = ptr(slugNamespace(req.ID))
	if args.Theme != nil {
		state.EffectiveTheme = *args.Theme
	}
	if req.DryRun {
		return infer.UpdateResponse[CollectionState]{Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.UpdateResponse[CollectionState]{}, err
	}
	patch := map[string]any{}
	if old.Title != args.Title {
		patch["title"] = args.Title
	}
	if ptrChanged(old.Description, args.Description) {
		patch["description"] = *args.Description
	}
	if orDefault(old.Private, false) != orDefault(args.Private, false) {
		patch["private"] = orDefault(args.Private, false)
	}
	if args.Theme != nil && *args.Theme != old.EffectiveTheme {
		patch["theme"] = *args.Theme
	}
	if len(patch) > 0 {
		if err := client.UpdateCollection(req.ID, patch); err != nil {
			return infer.UpdateResponse[CollectionState]{}, fmt.Errorf("update collection %q: %w", req.ID, err)
		}
	}
	return infer.UpdateResponse[CollectionState]{Output: state}, nil
}

func (r *Collection) Delete(ctx context.Context, req infer.DeleteRequest[CollectionState]) (infer.DeleteResponse, error) {
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteCollection(req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete collection %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getCollection
// ---------------------------------------------------------------------------

// GetCollection looks up a collection by slug.
type GetCollection struct{}

// GetCollectionArgs selects the collection.
type GetCollectionArgs struct {
	Token *string `pulumi:"token,optional" provider:"secret"`
	Slug  string  `pulumi:"slug"`
}

// CollectionItemSummary is an item of a looked-up collection.
type CollectionItemSummary struct {
	ItemObjectID string  `pulumi:"itemObjectId"`
	ItemID       string  `pulumi:"itemId"`
	ItemType     string  `pulumi:"itemType"`
	Note         *string `pulumi:"note,optional"`
	Position     int     `pulumi:"position"`
}

func (s *CollectionItemSummary) Annotate(a infer.Annotator) {
	a.Describe(&s.ItemObjectID, "The item's database ID (the last segment of a CollectionItem ID).")
	a.Describe(&s.ItemID, "The repository ID, paper ID or collection slug.")
	a.Describe(&s.ItemType, "The item type.")
	a.Describe(&s.Note, "The note on the item, when set.")
	a.Describe(&s.Position, "The item's position in the collection.")
}

// GetCollectionResult is the looked-up collection.
type GetCollectionResult struct {
	Slug        string                  `pulumi:"slug"`
	Title       string                  `pulumi:"title"`
	Description string                  `pulumi:"description"`
	Private     bool                    `pulumi:"private"`
	Theme       string                  `pulumi:"theme"`
	URL         string                  `pulumi:"url"`
	Items       []CollectionItemSummary `pulumi:"items"`
}

func (f *GetCollection) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Hub collection and its items by slug.")
}

func (a *GetCollectionArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.Slug, "The collection slug: 'namespace/title-slug-hash'.")
}

func (r *GetCollectionResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.Slug, "The collection slug.")
	ann.Describe(&r.Title, "The collection title.")
	ann.Describe(&r.Description, "The description (empty when unset).")
	ann.Describe(&r.Private, "Whether the collection is private.")
	ann.Describe(&r.Theme, "The colour theme.")
	ann.Describe(&r.URL, "The web URL of the collection.")
	ann.Describe(&r.Items, "The items in the collection.")
}

func (f *GetCollection) Invoke(ctx context.Context, req infer.FunctionRequest[GetCollectionArgs]) (infer.FunctionResponse[GetCollectionResult], error) {
	out := infer.FunctionResponse[GetCollectionResult]{}
	if req.Input.Slug == "" {
		return out, fmt.Errorf("huggingface: getCollection needs a slug")
	}
	client, _, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	col, err := client.GetCollection(req.Input.Slug)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up collection %q: %w", req.Input.Slug, err)
	}
	if col == nil {
		return out, fmt.Errorf("huggingface: collection %q was not found", req.Input.Slug)
	}
	items := make([]CollectionItemSummary, 0, len(col.Items))
	for _, it := range col.Items {
		s := CollectionItemSummary{ItemObjectID: it.ObjectID, ItemID: it.ItemID(), ItemType: it.Type, Position: it.Position}
		if t, ok := it.NoteText(); ok {
			s.Note = ptr(t)
		}
		items = append(items, s)
	}
	out.Output = GetCollectionResult{
		Slug: col.Slug, Title: col.Title, Description: col.Description, Private: col.Private,
		Theme: col.Theme, URL: collectionURL(col.Slug), Items: items,
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CollectionItem resource
// ---------------------------------------------------------------------------

// CollectionItem is one entry of a Collection.
type CollectionItem struct{}

var collectionItemTypes = []string{"model", "dataset", "space", "paper", "collection", "bucket"}

// CollectionItemArgs are the inputs of a CollectionItem.
type CollectionItemArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// CollectionSlug is the slug of the collection. Changing it replaces the item.
	CollectionSlug string `pulumi:"collectionSlug" provider:"replaceOnChanges"`
	// ItemID is the repo ID, paper ID or collection slug. Changing it replaces the item.
	ItemID string `pulumi:"itemId" provider:"replaceOnChanges"`
	// ItemType is the item type. Changing it replaces the item.
	ItemType string `pulumi:"itemType" provider:"replaceOnChanges"`
	// Note is a plain-text note shown with the item.
	Note *string `pulumi:"note,optional"`
}

// CollectionItemState is the stored state of a CollectionItem.
type CollectionItemState struct {
	CollectionItemArgs
	// ItemObjectID is the item's database ID, the last segment of the resource ID.
	ItemObjectID string `pulumi:"itemObjectId"`
}

func (r *CollectionItem) Annotate(a infer.Annotator) {
	a.Describe(&r, "An item of a Hugging Face Hub collection. The resource ID is '<collection slug>/<item object ID>' "+
		"(for example: pulumi import huggingface:index:CollectionItem i ns/recent-models-64f9a55bb3115b4f513ec026/64fa00aa11bb22cc33dd44ee). "+
		"The object ID is shown by getCollection.")
}

func (a *CollectionItemArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.CollectionSlug, "The collection slug. Changing it replaces the item.")
	ann.Describe(&a.ItemID, "The repository ID ('namespace/name'), paper ID or collection slug. Changing it replaces the item.")
	ann.Describe(&a.ItemType, "The item type: 'model', 'dataset', 'space', 'paper', 'collection' or 'bucket'. Changing it replaces the item.")
	ann.Describe(&a.Note, "A plain-text note shown with the item. Unmanaged while unset.")
}

func (s *CollectionItemState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.ItemObjectID, "The item's database ID.")
}

var (
	_ infer.CustomResource[CollectionItemArgs, CollectionItemState] = (*CollectionItem)(nil)
	_ infer.CustomCheck[CollectionItemArgs]                         = (*CollectionItem)(nil)
	_ infer.CustomDiff[CollectionItemArgs, CollectionItemState]     = (*CollectionItem)(nil)
	_ infer.CustomRead[CollectionItemArgs, CollectionItemState]     = (*CollectionItem)(nil)
	_ infer.CustomUpdate[CollectionItemArgs, CollectionItemState]   = (*CollectionItem)(nil)
	_ infer.CustomDelete[CollectionItemState]                       = (*CollectionItem)(nil)
)

func (r *CollectionItem) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[CollectionItemArgs], error) {
	args, failures, err := infer.DefaultCheck[CollectionItemArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[CollectionItemArgs]{Inputs: args, Failures: failures}, err
	}
	if !slices.Contains(collectionItemTypes, args.ItemType) {
		failures = append(failures, p.CheckFailure{Property: "itemType",
			Reason: fmt.Sprintf("invalid itemType %q; must be one of %v", args.ItemType, collectionItemTypes)})
	}
	if args.CollectionSlug == "" || !strings.Contains(args.CollectionSlug, "/") {
		failures = append(failures, p.CheckFailure{Property: "collectionSlug", Reason: "collectionSlug must look like 'namespace/title-slug-hash'"})
	}
	if args.ItemID == "" {
		failures = append(failures, p.CheckFailure{Property: "itemId", Reason: "itemId must not be empty"})
	}
	return infer.CheckResponse[CollectionItemArgs]{Inputs: args, Failures: dropUnknown(req.NewInputs, failures)}, nil
}

// splitItemID splits "<collection slug>/<item object ID>".
func splitItemID(id string) (slug, objectID string, err error) {
	i := strings.LastIndex(id, "/")
	if i <= 0 || i == len(id)-1 || !strings.Contains(id[:i], "/") {
		return "", "", fmt.Errorf("invalid collection item ID %q: expected <collection slug>/<item object ID>", id)
	}
	return id[:i], id[i+1:], nil
}

func findItem(col *CollectionDetails, match func(CollectionItemDetails) bool) *CollectionItemDetails {
	for i := range col.Items {
		if match(col.Items[i]) {
			return &col.Items[i]
		}
	}
	return nil
}

func (r *CollectionItem) Create(ctx context.Context, req infer.CreateRequest[CollectionItemArgs]) (infer.CreateResponse[CollectionItemState], error) {
	args := req.Inputs
	state := CollectionItemState{CollectionItemArgs: args}
	if req.DryRun {
		return infer.CreateResponse[CollectionItemState]{ID: req.Name, Output: state}, nil
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[CollectionItemState]{}, err
	}
	col, err := client.AddCollectionItem(args.CollectionSlug, args.ItemID, args.ItemType, args.Note)
	if err != nil {
		return infer.CreateResponse[CollectionItemState]{}, fmt.Errorf("add item %q to collection %q: %w", args.ItemID, args.CollectionSlug, err)
	}
	match := func(it CollectionItemDetails) bool { return it.Type == args.ItemType && it.ItemID() == args.ItemID }
	item := findItem(col, match)
	if item == nil {
		// Not in the response: look again before giving up.
		if again, gerr := client.GetCollection(args.CollectionSlug); gerr == nil && again != nil {
			item = findItem(again, match)
		}
	}
	if item == nil {
		return infer.CreateResponse[CollectionItemState]{}, fmt.Errorf("item %q was added to collection %q but is not listed in it", args.ItemID, args.CollectionSlug)
	}
	state.ItemObjectID = item.ObjectID
	return infer.CreateResponse[CollectionItemState]{ID: args.CollectionSlug + "/" + item.ObjectID, Output: state}, nil
}

func (r *CollectionItem) Read(ctx context.Context, req infer.ReadRequest[CollectionItemArgs, CollectionItemState]) (infer.ReadResponse[CollectionItemArgs, CollectionItemState], error) {
	type resp = infer.ReadResponse[CollectionItemArgs, CollectionItemState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	slug, objectID, err := splitItemID(req.ID)
	if err != nil {
		return resp{}, err
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	col, err := client.GetCollection(slug)
	if err != nil {
		return resp{}, fmt.Errorf("read collection item %q: %w", req.ID, err)
	}
	if col == nil {
		return resp{}, nil
	}
	item := findItem(col, func(it CollectionItemDetails) bool { return it.ObjectID == objectID })
	if item == nil {
		return resp{}, nil
	}
	imported := prior.CollectionSlug == ""
	inputs := CollectionItemArgs{Token: prior.Token, CollectionSlug: slug, ItemID: item.ItemID(), ItemType: item.Type}
	text, hasNote := item.NoteText()
	inputs.Note = refreshOpt(prior.Note, text, hasNote && (!imported || text != ""), imported, ptr(""))
	stateArgs := inputs
	if hasNote {
		stateArgs.Note = ptr(text)
	}
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  CollectionItemState{CollectionItemArgs: stateArgs, ItemObjectID: objectID},
	}, nil
}

func (r *CollectionItem) Diff(_ context.Context, req infer.DiffRequest[CollectionItemArgs, CollectionItemState]) (infer.DiffResponse, error) {
	o, n := req.State.CollectionItemArgs, req.Inputs
	d := newDiffSet()
	d.replace("collectionSlug", o.CollectionSlug != n.CollectionSlug)
	d.replace("itemId", o.ItemID != n.ItemID)
	d.replace("itemType", o.ItemType != n.ItemType)
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	d.update("note", ptrChanged(o.Note, n.Note))
	return d.response(), nil
}

func (r *CollectionItem) Update(ctx context.Context, req infer.UpdateRequest[CollectionItemArgs, CollectionItemState]) (infer.UpdateResponse[CollectionItemState], error) {
	args, old := req.Inputs, req.State
	state := CollectionItemState{CollectionItemArgs: args, ItemObjectID: old.ItemObjectID}
	if req.DryRun {
		return infer.UpdateResponse[CollectionItemState]{Output: state}, nil
	}
	slug, objectID, err := splitItemID(req.ID)
	if err != nil {
		return infer.UpdateResponse[CollectionItemState]{}, err
	}
	client, _, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.UpdateResponse[CollectionItemState]{}, err
	}
	if ptrChanged(old.Note, args.Note) {
		if err := client.UpdateCollectionItem(slug, objectID, map[string]any{"note": *args.Note}); err != nil {
			return infer.UpdateResponse[CollectionItemState]{}, fmt.Errorf("update collection item %q: %w", req.ID, err)
		}
	}
	return infer.UpdateResponse[CollectionItemState]{Output: state}, nil
}

func (r *CollectionItem) Delete(ctx context.Context, req infer.DeleteRequest[CollectionItemState]) (infer.DeleteResponse, error) {
	slug, objectID, err := splitItemID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteCollectionItem(slug, objectID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete collection item %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getCollectionItem
// ---------------------------------------------------------------------------

// GetCollectionItem looks up one item of a collection.
type GetCollectionItem struct{}

// GetCollectionItemArgs selects the item by collection and the item's own ID and type.
type GetCollectionItemArgs struct {
	Token          *string `pulumi:"token,optional" provider:"secret"`
	CollectionSlug string  `pulumi:"collectionSlug"`
	ItemID         string  `pulumi:"itemId"`
	ItemType       string  `pulumi:"itemType"`
}

// GetCollectionItemResult is the looked-up item.
type GetCollectionItemResult struct {
	CollectionSlug string  `pulumi:"collectionSlug"`
	ItemObjectID   string  `pulumi:"itemObjectId"`
	ItemID         string  `pulumi:"itemId"`
	ItemType       string  `pulumi:"itemType"`
	Note           *string `pulumi:"note,optional"`
	Position       int     `pulumi:"position"`
}

func (f *GetCollectionItem) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up one item of a Hugging Face Hub collection by its repository, paper or collection ID and type.")
}

func (a *GetCollectionItemArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.CollectionSlug, "The collection slug.")
	ann.Describe(&a.ItemID, "The repository ID, paper ID or collection slug of the item.")
	ann.Describe(&a.ItemType, "The item type.")
}

func (r *GetCollectionItemResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.CollectionSlug, "The collection slug.")
	ann.Describe(&r.ItemObjectID, "The item's database ID.")
	ann.Describe(&r.ItemID, "The repository ID, paper ID or collection slug.")
	ann.Describe(&r.ItemType, "The item type.")
	ann.Describe(&r.Note, "The note on the item, when set.")
	ann.Describe(&r.Position, "The item's position.")
}

func (f *GetCollectionItem) Invoke(ctx context.Context, req infer.FunctionRequest[GetCollectionItemArgs]) (infer.FunctionResponse[GetCollectionItemResult], error) {
	out := infer.FunctionResponse[GetCollectionItemResult]{}
	in := req.Input
	if in.CollectionSlug == "" || in.ItemID == "" || in.ItemType == "" {
		return out, fmt.Errorf("huggingface: getCollectionItem needs collectionSlug, itemId and itemType")
	}
	client, _, err := clientFor(ctx, in.Token)
	if err != nil {
		return out, err
	}
	col, err := client.GetCollection(in.CollectionSlug)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up collection %q: %w", in.CollectionSlug, err)
	}
	if col == nil {
		return out, fmt.Errorf("huggingface: collection %q was not found", in.CollectionSlug)
	}
	item := findItem(col, func(it CollectionItemDetails) bool { return it.Type == in.ItemType && it.ItemID() == in.ItemID })
	if item == nil {
		return out, fmt.Errorf("huggingface: %s %q was not found in collection %q", in.ItemType, in.ItemID, in.CollectionSlug)
	}
	res := GetCollectionItemResult{CollectionSlug: in.CollectionSlug, ItemObjectID: item.ObjectID, ItemID: item.ItemID(), ItemType: item.Type, Position: item.Position}
	if t, ok := item.NoteText(); ok {
		res.Note = ptr(t)
	}
	out.Output = res
	return out, nil
}
