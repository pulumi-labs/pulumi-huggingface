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
	"sort"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func kvEntry(k, v string) property.Value {
	return obj(map[string]property.Value{"key": str(k), "value": str(v)})
}

func spaceInputs() property.Map {
	return pm(map[string]property.Value{
		"sdk":       str("gradio"),
		"hardware":  str("cpu-basic"),
		"variables": arr(kvEntry("MODE", "demo"), kvEntry("REGION", "us")),
		"secrets":   arr(kvEntry("API_KEY", "s3cret"), kvEntry("OTHER", "x")),
	})
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSpace_CreateSendsConfiguration(t *testing.T) {
	l := newLC(t, "Space", "demo")
	in := with(spaceInputs(), map[string]property.Value{"private": boolean(true), "hardware": str("t4-small"), "sleepTimeSeconds": num(900)})
	c := l.create(in)
	require.Equal(t, "me/demo", c.ID)
	require.Equal(t, "me/demo", getStr(t, c.Properties, "spaceId"))
	require.Equal(t, "https://huggingface.co/spaces/me/demo", getStr(t, c.Properties, "url"))

	r := l.f.repo("space", "me", "demo")
	require.NotNil(t, r)
	require.True(t, r.private)
	require.Equal(t, "gradio", r.sdk)
	require.Equal(t, "t4-small", r.hardware)
	require.Equal(t, 900, *r.gcTimeout)
	require.Equal(t, []string{"MODE", "REGION"}, sortedKeys(r.variables))
	require.Equal(t, []string{"API_KEY", "OTHER"}, sortedKeys(r.secrets))
	require.Equal(t, "s3cret", r.secrets["API_KEY"]["value"])
}

func TestSpace_Lifecycle(t *testing.T) {
	l := newLC(t, "Space", "demo")
	inputs := spaceInputs()
	c := l.create(inputs)
	state := c.Properties

	// Idempotent diff, including reordered lists and explicit defaults.
	reordered := with(inputs, map[string]property.Value{
		"variables": arr(kvEntry("REGION", "us"), kvEntry("MODE", "demo")),
		"private":   boolean(false),
		"namespace": str("me"),
	})
	for _, in := range []property.Map{inputs, reordered, without(inputs, "hardware")} {
		requireNoChanges(t, l.diff(c.ID, state, in, inputs))
	}

	// Refresh keeps everything stable.
	rd := l.read(c.ID, state, inputs)
	require.Equal(t, "cpu-basic", getStr(t, rd.Properties, "hardware"))
	require.Equal(t, "gradio", getStr(t, rd.Inputs, "sdk"))
	require.Len(t, rd.Inputs.Get("variables").AsArray().AsSlice(), 2)
	require.Len(t, rd.Inputs.Get("secrets").AsArray().AsSlice(), 2)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, inputs, rd.Inputs))
	state = rd.Properties

	// Update: hardware, visibility, sleep time, variables and secrets.
	updated := with(inputs, map[string]property.Value{
		"hardware":         str("a10g-small"),
		"private":          boolean(true),
		"sleepTimeSeconds": num(600),
		"variables":        arr(kvEntry("MODE", "prod"), kvEntry("NEW", "1")), // MODE changed, REGION removed, NEW added
		"secrets":          arr(kvEntry("API_KEY", "rotated")),                // API_KEY changed, OTHER removed
	})
	d := l.diff(c.ID, state, updated, inputs)
	for _, prop := range []string{"hardware", "private", "sleepTimeSeconds", "variables", "secrets"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[prop].Kind, prop)
	}
	require.False(t, d.DeleteBeforeReplace)

	l.f.resetCalls()
	l.update(c.ID, state, updated, inputs)
	r := l.f.repo("space", "me", "demo")
	require.Equal(t, "a10g-small", r.hardware)
	require.True(t, r.private)
	require.Equal(t, 600, *r.gcTimeout)
	require.Equal(t, []string{"MODE", "NEW"}, sortedKeys(r.variables))
	require.Equal(t, "prod", r.variables["MODE"]["value"])
	require.Equal(t, []string{"API_KEY"}, sortedKeys(r.secrets))
	require.Equal(t, "rotated", r.secrets["API_KEY"]["value"])

	// Contract: one {key,value} object per POST, removal via DELETE {key}.
	for _, call := range l.f.callsMatching("POST", "/secrets") {
		b := decodeBody(t, call.Body)
		require.Contains(t, b, "key")
		require.Contains(t, b, "value")
	}
	require.Len(t, l.f.callsMatching("POST", "/variables"), 2)
	require.Len(t, l.f.callsMatching("POST", "/secrets"), 1)
	require.Equal(t, `{"key":"REGION"}`, l.f.callsMatching("DELETE", "/variables")[0].Body)
	require.Equal(t, `{"key":"OTHER"}`, l.f.callsMatching("DELETE", "/secrets")[0].Body)
	require.Len(t, l.f.callsMatching("PUT", "/api/spaces/me/demo/settings"), 1)
	require.Len(t, l.f.callsMatching("POST", "/api/spaces/me/demo/hardware"), 1)
	require.Equal(t, `{"seconds":600}`, l.f.callsMatching("POST", "/sleeptime")[0].Body)

	// Clearing the lists deletes every key (no silent no-op).
	cleared := without(updated, "variables", "secrets")
	st := l.read(c.ID, l.update(c.ID, state, updated, inputs).Properties, updated).Properties
	d = l.diff(c.ID, st, cleared, updated)
	require.Equal(t, pgp.Update, d.DetailedDiff["variables"].Kind)
	require.Equal(t, pgp.Update, d.DetailedDiff["secrets"].Kind)
	l.update(c.ID, st, cleared, updated)
	r = l.f.repo("space", "me", "demo")
	require.Empty(t, r.variables)
	require.Empty(t, r.secrets)

	// An unchanged update sends nothing.
	st = l.read(c.ID, st, cleared).Properties
	l.f.resetCalls()
	l.update(c.ID, st, cleared, cleared)
	for _, c := range l.f.calls {
		require.Equal(t, "GET", c.Method, "unexpected write %s %s", c.Method, c.Path)
	}
}

func TestSpace_ReplaceOnImmutableChanges(t *testing.T) {
	l := newLC(t, "Space", "demo")
	inputs := spaceInputs()
	c := l.create(inputs)

	d := l.diff(c.ID, c.Properties, with(inputs, map[string]property.Value{"sdk": str("docker")}), inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["sdk"].Kind)
	require.True(t, d.DeleteBeforeReplace)

	d = l.diff(c.ID, c.Properties, with(inputs, map[string]property.Value{"namespace": str("other-org")}), inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["namespace"].Kind)
	require.True(t, d.DeleteBeforeReplace)
}

func TestSpace_RefreshDetectsDrift(t *testing.T) {
	l := newLC(t, "Space", "demo")
	inputs := with(spaceInputs(), map[string]property.Value{"private": boolean(false)})
	c := l.create(inputs)

	// The real shape is runtime.hardware.current; the fake serves exactly that.
	l.f.mutateRepo("space", "me", "demo", func(r *fakeRepo) {
		r.hardware = "a100-large"
		r.private = true
		r.variables["MODE"]["value"] = "tampered"
		delete(r.variables, "REGION")
		delete(r.secrets, "OTHER")
		two := 120
		r.gcTimeout = &two
	})

	rd := l.read(c.ID, c.Properties, inputs)
	require.Equal(t, "a100-large", getStr(t, rd.Properties, "hardware"))
	require.Equal(t, "a100-large", getStr(t, rd.Inputs, "hardware"))
	require.True(t, rd.Inputs.Get("private").AsBool(), "a refreshed private lands in inputs so up can revert it")
	require.True(t, rd.Properties.Get("private").AsBool())
	vars := rd.Inputs.Get("variables").AsArray().AsSlice()
	require.Len(t, vars, 1)
	require.Equal(t, "tampered", vars[0].AsMap().Get("value").AsString())
	require.Len(t, rd.Inputs.Get("secrets").AsArray().AsSlice(), 1)

	d := l.diff(c.ID, rd.Properties, inputs, rd.Inputs)
	for _, prop := range []string{"hardware", "private", "variables", "secrets"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[prop].Kind, prop)
	}
	l.update(c.ID, rd.Properties, inputs, rd.Inputs)
	r := l.f.repo("space", "me", "demo")
	require.Equal(t, "cpu-basic", r.hardware)
	require.False(t, r.private)
	require.Equal(t, "demo", r.variables["MODE"]["value"])
	require.Contains(t, r.variables, "REGION")
	require.Contains(t, r.secrets, "OTHER")
}

func TestSpace_SecretValuesNeverComeBack(t *testing.T) {
	l := newLC(t, "Space", "demo")
	inputs := spaceInputs()
	c := l.create(inputs)
	rd := l.read(c.ID, c.Properties, inputs)
	secrets := rd.Properties.Get("secrets").AsArray().AsSlice()
	require.Len(t, secrets, 2)
	for _, s := range secrets {
		v := s.AsMap().Get("value")
		require.True(t, v.Secret(), "secret values stay marked secret")
	}
	// An import cannot recover values, so it adopts no secrets.
	l.f.addRepo("space", "me", "bare", func(r *fakeRepo) { r.secrets["K"] = map[string]any{"value": "v"} })
	imp := l.read("me/bare", property.NewMap(nil), property.NewMap(nil))
	require.False(t, hasProp(imp.Inputs, "secrets"))
}

func TestSpace_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "Space", "imported")
	two := 300
	l.f.addRepo("space", "me", "imported", func(r *fakeRepo) {
		r.sdk, r.hardware, r.private, r.gcTimeout = "docker", "t4-small", true, &two
		r.variables["B"] = map[string]any{"value": "2", "description": "second"}
		r.variables["A"] = map[string]any{"value": "1"}
	})
	rd := l.read("me/imported", property.NewMap(nil), property.NewMap(nil))
	require.Equal(t, "me/imported", rd.ID)
	require.Equal(t, "docker", getStr(t, rd.Inputs, "sdk"))
	require.Equal(t, "t4-small", getStr(t, rd.Inputs, "hardware"))
	require.Equal(t, "me", getStr(t, rd.Inputs, "namespace"))
	require.True(t, rd.Inputs.Get("private").AsBool())
	require.Equal(t, float64(300), rd.Inputs.Get("sleepTimeSeconds").AsNumber())
	vars := rd.Inputs.Get("variables").AsArray().AsSlice()
	require.Len(t, vars, 2)
	require.Equal(t, "A", vars[0].AsMap().Get("key").AsString())
	require.Equal(t, "second", vars[1].AsMap().Get("description").AsString())
	require.Equal(t, "https://huggingface.co/spaces/me/imported", getStr(t, rd.Properties, "url"))
	require.Equal(t, "me/imported", getStr(t, rd.Properties, "spaceId"))

	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: rd.Inputs})
	require.NoError(t, err)
	require.Empty(t, chk.Failures)
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, chk.Inputs, rd.Inputs))
}

func TestSpace_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "Space", "demo")
	inputs := spaceInputs()
	c := l.create(inputs)
	l.f.deleteRepoOutOfBand("space", "me", "demo")
	require.Empty(t, l.read(c.ID, c.Properties, inputs).ID)
	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
}

func TestSpace_Delete(t *testing.T) {
	l := newLC(t, "Space", "demo")
	c := l.create(spaceInputs())
	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
	require.Nil(t, l.f.repo("space", "me", "demo"))
}

func TestSpace_CheckValidation(t *testing.T) {
	l := newLC(t, "Space", "demo")
	cases := map[string]property.Map{
		"sdk":       with(spaceInputs(), map[string]property.Value{"sdk": str("flask")}),
		"hardware":  with(spaceInputs(), map[string]property.Value{"hardware": str("h100")}),
		"variables": with(spaceInputs(), map[string]property.Value{"variables": arr(kvEntry("A", "1"), kvEntry("A", "2"))}),
		"secrets":   with(spaceInputs(), map[string]property.Value{"secrets": arr(kvEntry("A", "1"), kvEntry("A", "2"))}),
	}
	for prop, in := range cases {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: in})
		require.NoError(t, err)
		require.NotEmpty(t, chk.Failures, prop)
		require.Equal(t, prop, chk.Failures[0].Property)
	}
	for _, hw := range []string{"cpu-basic", "l40sx1", "a100x8", "rtx-pro-6000"} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: with(spaceInputs(), map[string]property.Value{"hardware": str(hw)})})
		require.NoError(t, err)
		require.Empty(t, chk.Failures, hw)
	}
}

func TestSpace_PreviewDoesNotCallTheAPI(t *testing.T) {
	l := newLC(t, "Space", "demo")
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: spaceInputs(), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, c.ID, "a preview has no real ID, so the id output stays unknown (#1590)")
	require.Empty(t, l.f.calls)
	real := l.create(spaceInputs())
	l.f.resetCalls()
	u, err := l.s.Update(pgp.UpdateRequest{ID: real.ID, Urn: l.urn, State: real.Properties, Inputs: with(spaceInputs(), map[string]property.Value{"private": boolean(true)}), DryRun: true})
	require.NoError(t, err)
	require.True(t, u.Properties.Get("private").AsBool())
	require.Empty(t, l.f.calls)
}

func TestSpace_ReadWithoutTokenIsAnError(t *testing.T) {
	f := startFakeHF(t)
	s := serverFor(t, "")
	_, err := s.Read(pgp.ReadRequest{ID: "me/demo", Urn: hfUrn("Space", "demo"), Properties: spaceInputs(), Inputs: spaceInputs()})
	require.Error(t, err)
	require.Empty(t, f.calls)
}
