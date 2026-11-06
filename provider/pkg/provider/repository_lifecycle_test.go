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
	"encoding/json"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func TestRepository_ModelLifecycle(t *testing.T) {
	l := newLC(t, "Repository", "mymodel")
	inputs := pm(map[string]property.Value{})
	c := l.create(inputs)
	require.Equal(t, "me/mymodel", c.ID, "models keep the plain namespace/name ID")
	require.Equal(t, "me/mymodel", getStr(t, c.Properties, "repoId"))
	require.Equal(t, "https://huggingface.co/me/mymodel", getStr(t, c.Properties, "url"))
	require.Equal(t, "model", getStr(t, c.Properties, "repoType"))
	require.NotNil(t, l.f.repo("model", "me", "mymodel"))

	// Idempotent diff with and without explicit defaults.
	for _, in := range []property.Map{
		inputs,
		pm(map[string]property.Value{"repoType": str("model"), "private": boolean(false), "namespace": str("me")}),
	} {
		requireNoChanges(t, l.diff(c.ID, c.Properties, in, inputs))
	}

	// Refresh: stable.
	rd := l.read(c.ID, c.Properties, inputs)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, inputs, rd.Inputs))

	// Visibility is updated through the settings endpoint.
	priv := pm(map[string]property.Value{"private": boolean(true)})
	d := l.diff(c.ID, rd.Properties, priv, inputs)
	require.Equal(t, pgp.Update, d.DetailedDiff["private"].Kind)
	require.False(t, d.DeleteBeforeReplace)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, priv, inputs)
	require.True(t, l.f.repo("model", "me", "mymodel").private)
	puts := l.f.callsMatching("PUT", "/api/models/me/mymodel/settings")
	require.Len(t, puts, 1)
	require.Equal(t, `{"private":true}`, puts[0].Body)
	requireNoChanges(t, l.diff(c.ID, u.Properties, priv, priv))

	// ...and back.
	l.update(c.ID, u.Properties, inputs, priv)
	require.False(t, l.f.repo("model", "me", "mymodel").private)

	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
	require.Nil(t, l.f.repo("model", "me", "mymodel"))
}

func TestRepository_DatasetLifecycle(t *testing.T) {
	l := newLC(t, "Repository", "mydata")
	inputs := pm(map[string]property.Value{"repoType": str("dataset"), "private": boolean(true)})
	c := l.create(inputs)
	// The type is part of the ID so a dataset can be imported by ID alone.
	require.Equal(t, "datasets/me/mydata", c.ID)
	require.Equal(t, "me/mydata", getStr(t, c.Properties, "repoId"))
	require.Equal(t, "https://huggingface.co/datasets/me/mydata", getStr(t, c.Properties, "url"))
	require.NotNil(t, l.f.repo("dataset", "me", "mydata"))
	require.Nil(t, l.f.repo("model", "me", "mydata"))
	requireNoChanges(t, l.diff(c.ID, c.Properties, inputs, inputs))

	rd := l.read(c.ID, c.Properties, inputs)
	require.Equal(t, c.ID, rd.ID)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, inputs, rd.Inputs))

	pub := pm(map[string]property.Value{"repoType": str("dataset")})
	l.f.resetCalls()
	l.update(c.ID, rd.Properties, pub, inputs)
	require.False(t, l.f.repo("dataset", "me", "mydata").private)
	require.Len(t, l.f.callsMatching("PUT", "/api/datasets/me/mydata/settings"), 1)

	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
	require.Nil(t, l.f.repo("dataset", "me", "mydata"))
}

func TestRepository_ReplaceOnImmutableChanges(t *testing.T) {
	l := newLC(t, "Repository", "r")
	inputs := pm(map[string]property.Value{"private": boolean(true)})
	c := l.create(inputs)

	d := l.diff(c.ID, c.Properties, pm(map[string]property.Value{"private": boolean(true), "repoType": str("dataset")}), inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["repoType"].Kind)
	require.True(t, d.DeleteBeforeReplace)

	d = l.diff(c.ID, c.Properties, pm(map[string]property.Value{"private": boolean(true), "namespace": str("other")}), inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["namespace"].Kind)
	require.True(t, d.DeleteBeforeReplace)
}

func TestRepository_RefreshDetectsDriftAndRepairsIt(t *testing.T) {
	l := newLC(t, "Repository", "r")
	for _, typ := range []string{"model", "dataset"} {
		name := "drift-" + typ
		l.urn = hfUrn("Repository", name)
		inputs := pm(map[string]property.Value{"repoType": str(typ), "private": boolean(false)})
		c := l.create(inputs)

		l.f.mutateRepo(typ, "me", name, func(r *fakeRepo) { r.private = true })
		rd := l.read(c.ID, c.Properties, inputs)
		require.True(t, rd.Properties.Get("private").AsBool(), typ)
		require.True(t, rd.Inputs.Get("private").AsBool(), "%s: refreshed input lets up revert the change", typ)

		d := l.diff(c.ID, rd.Properties, inputs, rd.Inputs)
		require.Equal(t, pgp.Update, d.DetailedDiff["private"].Kind, typ)
		l.update(c.ID, rd.Properties, inputs, rd.Inputs)
		require.False(t, l.f.repo(typ, "me", name).private, typ)
	}
}

func TestRepository_DriftWhenPrivateWasUnset(t *testing.T) {
	l := newLC(t, "Repository", "r")
	inputs := pm(map[string]property.Value{})
	c := l.create(inputs)
	l.f.mutateRepo("model", "me", "r", func(r *fakeRepo) { r.private = true })
	rd := l.read(c.ID, c.Properties, inputs)
	d := l.diff(c.ID, rd.Properties, inputs, rd.Inputs)
	require.Equal(t, pgp.Update, d.DetailedDiff["private"].Kind, "unset means public")
}

func TestRepository_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "Repository", "imp")
	l.f.addRepo("model", "acme", "m", func(r *fakeRepo) { r.private = true })
	l.f.addRepo("dataset", "acme", "d", nil)

	for _, tc := range []struct{ id, typ, repoID, url string }{
		{"acme/m", "model", "acme/m", "https://huggingface.co/acme/m"},
		{"datasets/acme/d", "dataset", "acme/d", "https://huggingface.co/datasets/acme/d"},
	} {
		rd := l.read(tc.id, property.NewMap(nil), property.NewMap(nil))
		require.Equal(t, tc.id, rd.ID, tc.id)
		require.Equal(t, tc.typ, getStr(t, rd.Inputs, "repoType"), tc.id)
		require.Equal(t, "acme", getStr(t, rd.Inputs, "namespace"), tc.id)
		require.Equal(t, tc.repoID, getStr(t, rd.Properties, "repoId"), tc.id)
		require.Equal(t, tc.url, getStr(t, rd.Properties, "url"), tc.id)

		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: rd.Inputs})
		require.NoError(t, err)
		require.Empty(t, chk.Failures)
		requireNoChanges(t, l.diff(rd.ID, rd.Properties, chk.Inputs, rd.Inputs))
	}

	// The type prefix decides where to look: a dataset is not found as a model.
	require.Empty(t, l.read("acme/d", property.NewMap(nil), property.NewMap(nil)).ID)
}

func TestRepository_LegacyDatasetIDStillResolvesThroughState(t *testing.T) {
	// Earlier versions gave datasets the plain namespace/name ID and recorded
	// repoType in state; those IDs must keep working.
	l := newLC(t, "Repository", "legacy")
	l.f.addRepo("dataset", "me", "legacy", func(r *fakeRepo) { r.private = true })
	state := pm(map[string]property.Value{
		"repoType": str("dataset"), "private": boolean(true), "namespace": str("me"),
		"repoId": str("me/legacy"), "url": str("https://huggingface.co/datasets/me/legacy"),
	})
	inputs := pm(map[string]property.Value{"repoType": str("dataset"), "private": boolean(true)})
	rd := l.read("me/legacy", state, inputs)
	require.Equal(t, "me/legacy", rd.ID)
	requireNoChanges(t, l.diff("me/legacy", rd.Properties, inputs, rd.Inputs))

	l.f.resetCalls()
	l.update("me/legacy", rd.Properties, pm(map[string]property.Value{"repoType": str("dataset")}), inputs)
	require.False(t, l.f.repo("dataset", "me", "legacy").private)

	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: "me/legacy", Urn: l.urn, Properties: rd.Properties}))
	require.Nil(t, l.f.repo("dataset", "me", "legacy"))
}

func TestRepository_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "Repository", "gone")
	inputs := pm(map[string]property.Value{"repoType": str("dataset")})
	c := l.create(inputs)
	l.f.deleteRepoOutOfBand("dataset", "me", "gone")
	require.Empty(t, l.read(c.ID, c.Properties, inputs).ID)
	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))

	m := newLC(t, "Repository", "gonemodel")
	mc := m.create(pm(map[string]property.Value{}))
	m.f.deleteRepoOutOfBand("model", "me", "gonemodel")
	require.NoError(t, m.s.Delete(pgp.DeleteRequest{ID: mc.ID, Urn: m.urn, Properties: mc.Properties}))
}

func TestRepository_CheckRepoType(t *testing.T) {
	l := newLC(t, "Repository", "r")
	for _, bad := range []string{"space", "bucket"} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(map[string]property.Value{"repoType": str(bad)})})
		require.NoError(t, err)
		require.Len(t, chk.Failures, 1, bad)
		require.Equal(t, "repoType", chk.Failures[0].Property)
	}
	for _, ok := range []string{"model", "dataset"} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(map[string]property.Value{"repoType": str(ok)})})
		require.NoError(t, err)
		require.Empty(t, chk.Failures, ok)
	}
}

func TestRepository_PreviewAndMissingToken(t *testing.T) {
	l := newLC(t, "Repository", "r")
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: pm(map[string]property.Value{}), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, c.ID, "a preview has no real ID, so the id output stays unknown (#1590)")
	require.Empty(t, l.f.calls)

	s := serverFor(t, "")
	l.f.resetCalls()
	_, err = s.Read(pgp.ReadRequest{ID: "me/r", Urn: l.urn, Properties: pm(map[string]property.Value{"private": boolean(true)}), Inputs: pm(map[string]property.Value{})})
	require.Error(t, err)
	require.Empty(t, l.f.calls)
}

func TestSplitID_RejectsEmptySegments(t *testing.T) {
	for _, id := range []string{"/name", "ns/", "/"} {
		_, _, err := SplitID(id)
		require.Error(t, err, id)
	}
}
