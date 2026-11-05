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

import "github.com/pulumi/pulumi-go-provider/infer"

// By default infer marks every output computed during an update preview when
// any input changed. Most Hub outputs are identifiers fixed at creation, so a
// preview of a description or visibility change would show them as unknown and
// ripple "known after apply" into every stack output and resource that
// reads them (#1550). Each resource below declares what its outputs really
// derive from. Declaring any dependency turns the defaults off for all of the
// resource's outputs, and an output that no input drives is known on update.
//
// The identifier outputs are wired to the token only. They are fixed by the
// namespace and name, but infer compares the stored state with the new inputs
// to decide what changed, and the namespace is stored resolved while the input
// is usually unset, so a namespace dependency would flag them as changed on
// every update. Namespace, name and repo type changes replace the resource
// (a create, where the output is the new value), so the only in-place change
// that could move an identifier is a token swap. Tokens are compared as stored
// against as given, so an unchanged token keeps the identifiers known.

// WireDependencies: the slug and URL are fixed at creation; the Hub's theme
// follows the theme input.
func (r *Collection) WireDependencies(f infer.FieldSelector, args *CollectionArgs, state *CollectionState) {
	f.OutputField(&state.Slug).DependsOn(f.InputField(&args.Token).Computed())
	f.OutputField(&state.URL).DependsOn(f.InputField(&args.Token).Computed())
	f.OutputField(&state.EffectiveTheme).DependsOn(f.InputField(&args.Theme).Computed())
}

// WireDependencies: the ID and URL are fixed at creation (see the note above on why they are wired to the token).
func (r *Repository) WireDependencies(f infer.FieldSelector, args *RepositoryArgs, state *RepositoryState) {
	f.OutputField(&state.RepoID).DependsOn(f.InputField(&args.Token).Computed())
	f.OutputField(&state.URL).DependsOn(f.InputField(&args.Token).Computed())
}

// WireDependencies: the ID and URL are fixed at creation (wired to the token, see above).
func (r *Space) WireDependencies(f infer.FieldSelector, args *SpaceArgs, state *SpaceState) {
	f.OutputField(&state.SpaceID).DependsOn(f.InputField(&args.Token).Computed())
	f.OutputField(&state.URL).DependsOn(f.InputField(&args.Token).Computed())
}

// WireDependencies: the ID and URL are fixed at creation (wired to the token, see above).
func (r *Bucket) WireDependencies(f infer.FieldSelector, args *BucketArgs, state *BucketState) {
	f.OutputField(&state.BucketID).DependsOn(f.InputField(&args.Token).Computed())
	f.OutputField(&state.URL).DependsOn(f.InputField(&args.Token).Computed())
}

// WireDependencies: the item's object ID is fixed by what it points at.
func (r *CollectionItem) WireDependencies(f infer.FieldSelector, args *CollectionItemArgs, state *CollectionItemState) {
	f.OutputField(&state.ItemObjectID).DependsOn(
		f.InputField(&args.CollectionSlug).Computed(), f.InputField(&args.ItemID).Computed(), f.InputField(&args.ItemType).Computed())
}

// WireDependencies: the domains the Hub applies follow the domains input.
func (r *Webhook) WireDependencies(f infer.FieldSelector, args *WebhookArgs, state *WebhookState) {
	f.OutputField(&state.EffectiveDomains).DependsOn(f.InputField(&args.Domains).Computed())
}
