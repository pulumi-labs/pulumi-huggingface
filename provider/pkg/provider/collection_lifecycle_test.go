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
	"strings"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func collectionInputs() property.Map {
	return pm(map[string]property.Value{
		"title": str("Recent Models"), "description": str("What we ship"), "private": boolean(true),
	})
}

func TestCollection_Lifecycle(t *testing.T) {
	l := newLC(t, "Collection", "col")
	in := collectionInputs()
	c := l.create(in)
	require.True(t, strings.HasPrefix(c.ID, "me/recent-models-"), c.ID)
	require.Equal(t, c.ID, getStr(t, c.Properties, "slug"))
	require.Equal(t, "https://huggingface.co/collections/"+c.ID, getStr(t, c.Properties, "url"))
	require.Equal(t, "me", getStr(t, c.Properties, "namespace"))
	stored := l.f.collection(c.ID)
	require.Equal(t, "Recent Models", stored.title)
	require.True(t, stored.private)
	require.Equal(t, "What we ship", stored.description)
	body := decodeBody(t, l.f.callsMatching("POST", "/api/collections")[0].Body)
	require.Equal(t, "me", body["namespace"])
	require.NotContains(t, body, "theme", "the create call has no theme")

	// The Hub picks a theme: an output, not an input.
	require.Equal(t, "orange", getStr(t, c.Properties, "effectiveTheme"))
	require.False(t, hasProp(c.Properties, "theme"))

	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))
	require.False(t, hasProp(rd.Inputs, "theme"))

	next := with(in, map[string]property.Value{
		"title": str("Models 2026"), "description": str("New"), "private": boolean(false), "theme": str("green"),
	})
	d := l.diff(c.ID, rd.Properties, next, in)
	for _, k := range []string{"title", "description", "private", "theme"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[k].Kind, k)
	}
	require.False(t, d.DeleteBeforeReplace)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, next, in)
	stored = l.f.collection(c.ID)
	require.Equal(t, "Models 2026", stored.title)
	require.False(t, stored.private)
	require.Equal(t, "New", stored.description)
	require.Equal(t, "green", stored.theme)
	require.Len(t, l.f.callsMatching("PATCH", "/api/collections/"+c.ID), 1)
	require.Equal(t, "green", getStr(t, u.Properties, "effectiveTheme"))
	requireNoChanges(t, l.diff(c.ID, u.Properties, next, next))

	// Namespace is immutable.
	d = l.diff(c.ID, u.Properties, with(next, map[string]property.Value{"namespace": str("acme")}), next)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["namespace"].Kind)
	require.True(t, d.DeleteBeforeReplace)

	l.del(c.ID, u.Properties)
	require.Nil(t, l.f.collection(c.ID))
	l.del(c.ID, u.Properties)
}

func TestCollection_CreateWithThemeAndNamespace(t *testing.T) {
	l := newLC(t, "Collection", "col")
	in := with(collectionInputs(), map[string]property.Value{"theme": str("purple"), "namespace": str("acme")})
	c := l.create(in)
	require.True(t, strings.HasPrefix(c.ID, "acme/"), c.ID)
	require.Equal(t, "purple", l.f.collection(c.ID).theme)
	require.Equal(t, "purple", getStr(t, c.Properties, "effectiveTheme"))
	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
}

func TestCollection_RefreshDetectsDriftAndRepairsIt(t *testing.T) {
	l := newLC(t, "Collection", "col")
	in := collectionInputs()
	c := l.create(in)
	l.f.mutateCollection(c.ID, func(c *fakeCollection) { c.title = "Renamed"; c.private = false; c.description = "" })
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, "Renamed", getStr(t, rd.Inputs, "title"))
	d := l.diff(c.ID, rd.Properties, in, rd.Inputs)
	for _, k := range []string{"title", "private", "description"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[k].Kind, k)
	}
	l.update(c.ID, rd.Properties, in, rd.Inputs)
	stored := l.f.collection(c.ID)
	require.Equal(t, "Recent Models", stored.title)
	require.True(t, stored.private)
	require.Equal(t, "What we ship", stored.description)
}

func TestCollection_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "Collection", "imp")
	slug := l.f.addCollection("acme", "Imported")
	l.f.mutateCollection(slug, func(c *fakeCollection) { c.description = "d"; c.private = true })
	rd := l.read(slug, empty(), empty())
	require.Equal(t, slug, rd.ID)
	require.Equal(t, "Imported", getStr(t, rd.Inputs, "title"))
	require.Equal(t, "acme", getStr(t, rd.Inputs, "namespace"))
	require.Equal(t, "d", getStr(t, rd.Inputs, "description"))
	require.True(t, rd.Inputs.Get("private").AsBool())
	require.Equal(t, "blue", getStr(t, rd.Inputs, "theme"))
	in := l.checked(rd.Inputs)
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, in, rd.Inputs))
}

func TestCollection_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "Collection", "gone")
	in := collectionInputs()
	c := l.create(in)
	l.f.deleteCollectionOutOfBand(c.ID)
	require.Empty(t, l.read(c.ID, c.Properties, in).ID)
	l.del(c.ID, c.Properties)
}

func TestCollection_CheckAndFailureAfterCreate(t *testing.T) {
	l := newLC(t, "Collection", "col")
	require.NotEmpty(t, l.checkFailures(pm(map[string]property.Value{"title": str("  ")})))

	l.f.failOn = []string{"PATCH /api/collections"}
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.checked(with(collectionInputs(), map[string]property.Value{"theme": str("pink")}))})
	require.Error(t, err)
	require.NotEmpty(t, c.ID, "the collection exists, so it is recorded with its ID")
	require.NotNil(t, l.f.collection(c.ID))
	require.False(t, hasProp(c.Properties, "theme"))
}

func TestCollection_Lookup(t *testing.T) {
	l := newLC(t, "Collection", "col")
	c := l.create(collectionInputs())
	l.f.addCollectionItem(c.ID, "acme/m", "model")
	out, err := invoke(t, l.s, "getCollection", map[string]property.Value{"slug": str(c.ID)})
	require.NoError(t, err)
	require.Equal(t, "Recent Models", getStr(t, out, "title"))
	items := out.Get("items").AsArray().AsSlice()
	require.Len(t, items, 1)
	require.Equal(t, "acme/m", getStr(t, items[0].AsMap(), "itemId"))
	require.NotEmpty(t, getStr(t, items[0].AsMap(), "itemObjectId"))

	_, err = invoke(t, l.s, "getCollection", map[string]property.Value{"slug": str("me/nope-123")})
	require.ErrorContains(t, err, `collection "me/nope-123" was not found`)
}

// ---------------------------------------------------------------------------
// CollectionItem
// ---------------------------------------------------------------------------

func itemInputs(slug string) property.Map {
	return pm(map[string]property.Value{
		"collectionSlug": str(slug), "itemId": str("acme/m"), "itemType": str("model"), "note": str("our flagship"),
	})
}

func TestCollectionItem_Lifecycle(t *testing.T) {
	l := newLC(t, "CollectionItem", "item")
	slug := l.f.addCollection("me", "Models")
	in := itemInputs(slug)
	c := l.create(in)
	oid := getStr(t, c.Properties, "itemObjectId")
	require.Equal(t, slug+"/"+oid, c.ID)
	items := l.f.collection(slug).items
	require.Len(t, items, 1)
	require.Equal(t, "acme/m", items[0].id)
	require.Equal(t, "our flagship", *items[0].note)
	body := decodeBody(t, l.f.callsMatching("POST", "/items")[0].Body)
	require.Equal(t, "model", dig(body, "item", "type"))
	require.Equal(t, "our flagship", body["note"])

	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, c.ID, rd.ID)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// The note is the only mutable property.
	next := with(in, map[string]property.Value{"note": str("updated")})
	d := l.diff(c.ID, rd.Properties, next, in)
	require.Equal(t, pgp.Update, d.DetailedDiff["note"].Kind)
	require.False(t, d.DeleteBeforeReplace)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, next, in)
	require.Equal(t, "updated", *l.f.collection(slug).items[0].note)
	require.Len(t, l.f.callsMatching("PATCH", "/items/"+oid), 1)
	requireNoChanges(t, l.diff(c.ID, u.Properties, next, next))

	for k, v := range map[string]property.Value{"itemId": str("acme/other"), "itemType": str("dataset"), "collectionSlug": str("me/other-1")} {
		d = l.diff(c.ID, u.Properties, with(next, map[string]property.Value{k: v}), next)
		require.Equal(t, pgp.UpdateReplace, d.DetailedDiff[k].Kind, k)
		require.True(t, d.DeleteBeforeReplace)
	}

	l.del(c.ID, u.Properties)
	require.Empty(t, l.f.collection(slug).items)
	l.del(c.ID, u.Properties)
}

func TestCollectionItem_CollectionItemsUseTheSlug(t *testing.T) {
	l := newLC(t, "CollectionItem", "item")
	slug := l.f.addCollection("me", "Models")
	other := l.f.addCollection("acme", "Other")
	in := pm(map[string]property.Value{"collectionSlug": str(slug), "itemId": str(other), "itemType": str("collection")})
	c := l.create(in)
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, other, getStr(t, rd.Inputs, "itemId"))
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))
}

func TestCollectionItem_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "CollectionItem", "imp")
	slug := l.f.addCollection("me", "Models")
	oid := l.f.addCollectionItem(slug, "acme/ds", "dataset")
	rd := l.read(slug+"/"+oid, empty(), empty())
	require.Equal(t, slug+"/"+oid, rd.ID)
	require.Equal(t, slug, getStr(t, rd.Inputs, "collectionSlug"))
	require.Equal(t, "acme/ds", getStr(t, rd.Inputs, "itemId"))
	require.Equal(t, "dataset", getStr(t, rd.Inputs, "itemType"))
	require.False(t, hasProp(rd.Inputs, "note"))
	in := l.checked(rd.Inputs)
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, in, rd.Inputs))

	_, err := l.s.Read(pgp.ReadRequest{ID: "no-slash", Urn: l.urn, Properties: empty(), Inputs: empty()})
	require.Error(t, err)
}

func TestCollectionItem_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "CollectionItem", "gone")
	slug := l.f.addCollection("me", "Models")
	in := itemInputs(slug)
	c := l.create(in)
	l.f.mutateCollection(slug, func(c *fakeCollection) { c.items = nil })
	require.Empty(t, l.read(c.ID, c.Properties, in).ID, "item removed from its collection")
	l.del(c.ID, c.Properties)

	c = l.create(in)
	l.f.deleteCollectionOutOfBand(slug)
	require.Empty(t, l.read(c.ID, c.Properties, in).ID, "collection deleted")
	l.del(c.ID, c.Properties)
}

func TestCollectionItem_CheckAndDuplicate(t *testing.T) {
	l := newLC(t, "CollectionItem", "item")
	slug := l.f.addCollection("me", "Models")
	require.NotEmpty(t, l.checkFailures(with(itemInputs(slug), map[string]property.Value{"itemType": str("video")})))
	require.NotEmpty(t, l.checkFailures(with(itemInputs(slug), map[string]property.Value{"collectionSlug": str("noslash")})))
	require.NotEmpty(t, l.checkFailures(with(itemInputs(slug), map[string]property.Value{"itemId": str("")})))

	l.create(itemInputs(slug))
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: itemInputs(slug)})
	require.Error(t, err, "the Hub answers 409 for an item already in the collection")
}

func TestCollectionItem_Lookup(t *testing.T) {
	l := newLC(t, "CollectionItem", "item")
	slug := l.f.addCollection("me", "Models")
	l.f.addCollectionItem(slug, "acme/m", "model")
	out, err := invoke(t, l.s, "getCollectionItem", map[string]property.Value{
		"collectionSlug": str(slug), "itemId": str("acme/m"), "itemType": str("model")})
	require.NoError(t, err)
	require.NotEmpty(t, getStr(t, out, "itemObjectId"))

	_, err = invoke(t, l.s, "getCollectionItem", map[string]property.Value{
		"collectionSlug": str(slug), "itemId": str("acme/zz"), "itemType": str("model")})
	require.ErrorContains(t, err, `model "acme/zz" was not found in collection`)
	_, err = invoke(t, l.s, "getCollectionItem", map[string]property.Value{
		"collectionSlug": str("me/gone-1"), "itemId": str("acme/zz"), "itemType": str("model")})
	require.ErrorContains(t, err, `collection "me/gone-1" was not found`)
}
