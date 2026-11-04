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
	"fmt"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestLookup_Whoami(t *testing.T) {
	l := newLC(t, "Space", "x")
	out, err := invoke(t, l.s, "getWhoami", nil)
	require.NoError(t, err)
	require.Equal(t, "me", getStr(t, out, "name"))
	require.Equal(t, "user", getStr(t, out, "type"))
	require.Equal(t, "Me Myself", getStr(t, out, "fullName"))
	require.Equal(t, []string{"acme"}, strs(out, "orgs"))

	// Without a token the function reports it instead of calling the API.
	s := serverFor(t, "")
	l.f.resetCalls()
	_, err = invoke(t, s, "getWhoami", nil)
	require.Error(t, err)
	require.Empty(t, l.f.calls)
}

func TestLookup_Repository(t *testing.T) {
	l := newLC(t, "Repository", "x")
	l.f.addRepo("model", "acme", "m", func(r *fakeRepo) { r.private = true; r.gated = "manual" })
	l.f.addRepo("dataset", "acme", "d", nil)

	out, err := invoke(t, l.s, "getRepository", map[string]property.Value{"repoId": str("acme/m")})
	require.NoError(t, err)
	require.Equal(t, "acme/m", getStr(t, out, "repoId"))
	require.Equal(t, "model", getStr(t, out, "repoType"))
	require.True(t, out.Get("private").AsBool())
	require.Equal(t, "manual", getStr(t, out, "gated"))
	require.Equal(t, "https://huggingface.co/acme/m", getStr(t, out, "url"))

	out, err = invoke(t, l.s, "getRepository", map[string]property.Value{"repoId": str("acme/d"), "repoType": str("dataset")})
	require.NoError(t, err)
	require.Equal(t, "off", getStr(t, out, "gated"))
	require.Equal(t, "https://huggingface.co/datasets/acme/d", getStr(t, out, "url"))

	_, err = invoke(t, l.s, "getRepository", map[string]property.Value{"repoId": str("acme/d")})
	require.ErrorContains(t, err, `model repository "acme/d" was not found`, "the type decides where to look")
	_, err = invoke(t, l.s, "getRepository", map[string]property.Value{"repoId": str("acme/m"), "repoType": str("space")})
	require.Error(t, err)
	_, err = invoke(t, l.s, "getRepository", map[string]property.Value{"repoId": str("nonamespace")})
	require.Error(t, err)
}

func TestLookup_Space(t *testing.T) {
	l := newLC(t, "Space", "demo")
	in := with(spaceInputs(), map[string]property.Value{"sleepTimeSeconds": num(600), "hardware": str("t4-small")})
	l.create(in)

	out, err := invoke(t, l.s, "getSpace", map[string]property.Value{"spaceId": str("me/demo")})
	require.NoError(t, err)
	require.Equal(t, "me/demo", getStr(t, out, "spaceId"))
	require.Equal(t, "gradio", getStr(t, out, "sdk"))
	require.Equal(t, "t4-small", getStr(t, out, "hardware"))
	require.Equal(t, "RUNNING", getStr(t, out, "stage"))
	require.Equal(t, 600.0, out.Get("sleepTimeSeconds").AsNumber())
	require.Equal(t, "demo", out.Get("variables").AsMap().Get("MODE").AsString())
	// Secret values are write-only in the API: only the names come back.
	require.Equal(t, []string{"API_KEY", "OTHER"}, strs(out, "secretKeys"))
	require.NotContains(t, fmt.Sprint(out), "s3cret")

	_, err = invoke(t, l.s, "getSpace", map[string]property.Value{"spaceId": str("me/none")})
	require.ErrorContains(t, err, `space "me/none" was not found`)
}

func TestLookup_InferenceEndpointAndList(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	in := with(endpointInputs(), map[string]property.Value{"tags": arr(str("prod"), str("team-a"))})
	l.create(in)
	l.f.addEndpoint("acme/other", map[string]any{
		"name": "other", "type": "public",
		"compute":  map[string]any{"accelerator": "cpu", "instanceType": "intel-icl", "instanceSize": "x1", "scaling": map[string]any{"minReplica": 0, "maxReplica": 2, "scaleToZeroTimeout": 30}},
		"model":    map[string]any{"repository": "org/o", "framework": "pytorch", "task": "fill-mask", "revision": "abc"},
		"provider": map[string]any{"vendor": "aws", "region": "us-east-1"},
		"status":   map[string]any{"state": "scaledToZero", "message": "", "url": ""},
	})

	out, err := invoke(t, l.s, "getInferenceEndpoint", map[string]property.Value{"name": str("ep")})
	require.NoError(t, err)
	require.Equal(t, "ep", getStr(t, out, "name"))
	require.Equal(t, "me", getStr(t, out, "namespace"))
	require.Equal(t, "authenticated", getStr(t, out, "endpointType"))
	require.Equal(t, "running", getStr(t, out, "status"))
	require.Equal(t, "https://ep.endpoints.huggingface.cloud", getStr(t, out, "url"))
	require.Equal(t, "org/model", getStr(t, out, "modelRepository"))
	require.Equal(t, []string{"prod", "team-a"}, strs(out, "tags"))

	out, err = invoke(t, l.s, "getInferenceEndpoint", map[string]property.Value{"namespace": str("acme"), "name": str("other")})
	require.NoError(t, err)
	require.Equal(t, "scaledToZero", getStr(t, out, "status"))
	require.Equal(t, "abc", getStr(t, out, "modelRevision"))
	require.Equal(t, 30.0, out.Get("scaleToZeroTimeout").AsNumber())

	_, err = invoke(t, l.s, "getInferenceEndpoint", map[string]property.Value{"name": str("missing")})
	require.ErrorContains(t, err, `inference endpoint "me/missing" was not found`)

	out, err = invoke(t, l.s, "listInferenceEndpoints", nil)
	require.NoError(t, err)
	require.Equal(t, "me", getStr(t, out, "namespace"))
	require.Len(t, out.Get("endpoints").AsArray().AsSlice(), 1)
	out, err = invoke(t, l.s, "listInferenceEndpoints", map[string]property.Value{"namespace": str("acme")})
	require.NoError(t, err)
	eps := out.Get("endpoints").AsArray().AsSlice()
	require.Len(t, eps, 1)
	require.Equal(t, "other", getStr(t, eps[0].AsMap(), "name"))
	out, err = invoke(t, l.s, "listInferenceEndpoints", map[string]property.Value{"namespace": str("empty")})
	require.NoError(t, err)
	require.Empty(t, out.Get("endpoints").AsArray().AsSlice())
}

func TestLookup_RequireInput(t *testing.T) {
	l := newLC(t, "Space", "x")
	for tok, args := range map[string]map[string]property.Value{
		"getWebhook":           {"webhookId": str("")},
		"getCollection":        {"slug": str("")},
		"getBucket":            {"name": str("")},
		"getInferenceEndpoint": {"name": str("")},
		"getRepositoryTag":     {"repoId": str("bad"), "tag": str("v")},
	} {
		_, err := invoke(t, l.s, tok, args)
		require.Error(t, err, tok)
	}
}

// ---------------------------------------------------------------------------
// Repository gated access
// ---------------------------------------------------------------------------

func TestRepository_GatedLifecycle(t *testing.T) {
	l := newLC(t, "Repository", "gm")
	in := pm(map[string]property.Value{"gated": str("auto")})
	l.f.resetCalls()
	c := l.create(in)
	require.Equal(t, "auto", l.f.repo("model", "me", "gm").gated)
	puts := l.f.callsMatching("PUT", "/api/models/me/gm/settings")
	require.Len(t, puts, 1)
	require.Equal(t, `{"gated":"auto"}`, puts[0].Body)
	require.Equal(t, "auto", getStr(t, c.Properties, "gated"))
	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))

	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// auto -> manual together with private: one settings call with both.
	next := pm(map[string]property.Value{"gated": str("manual"), "private": boolean(true)})
	d := l.diff(c.ID, rd.Properties, next, in)
	require.Equal(t, pgp.Update, d.DetailedDiff["gated"].Kind)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, next, in)
	puts = l.f.callsMatching("PUT", "/settings")
	require.Len(t, puts, 1)
	require.Equal(t, map[string]any{"gated": "manual", "private": true}, decodeBody(t, puts[0].Body))
	require.Equal(t, "manual", l.f.repo("model", "me", "gm").gated)
	requireNoChanges(t, l.diff(c.ID, u.Properties, next, next))

	// Unset means off: removing the input turns gating off with gated=false.
	off := pm(map[string]property.Value{"private": boolean(true)})
	d = l.diff(c.ID, u.Properties, off, next)
	require.Equal(t, pgp.Update, d.DetailedDiff["gated"].Kind)
	l.f.resetCalls()
	l.update(c.ID, u.Properties, off, next)
	require.Equal(t, "", l.f.repo("model", "me", "gm").gated)
	require.Equal(t, `{"gated":false}`, l.f.callsMatching("PUT", "/settings")[0].Body)
}

func TestRepository_GatedDriftAndImport(t *testing.T) {
	l := newLC(t, "Repository", "gd")
	in := pm(map[string]property.Value{})
	c := l.create(in)
	require.Empty(t, l.f.callsMatching("PUT", "/settings"), "off is the default: no settings call")
	require.False(t, hasProp(c.Properties, "gated"), "unset stays unset in state; the effective value is off")

	l.f.mutateRepo("model", "me", "gd", func(r *fakeRepo) { r.gated = "auto" })
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, "auto", getStr(t, rd.Inputs, "gated"), "drift is surfaced")
	d := l.diff(c.ID, rd.Properties, in, rd.Inputs)
	require.Equal(t, pgp.Update, d.DetailedDiff["gated"].Kind)
	l.update(c.ID, rd.Properties, in, rd.Inputs)
	require.Equal(t, "", l.f.repo("model", "me", "gd").gated)

	l.f.addRepo("dataset", "acme", "gated-ds", func(r *fakeRepo) { r.gated = "manual" })
	imp := l.read("datasets/acme/gated-ds", empty(), empty())
	require.Equal(t, "manual", getStr(t, imp.Inputs, "gated"))
	chk := l.checked(imp.Inputs)
	requireNoChanges(t, l.diff(imp.ID, imp.Properties, chk, imp.Inputs))
}

func TestRepository_GatedCheckAndPartialFailure(t *testing.T) {
	l := newLC(t, "Repository", "g")
	require.NotEmpty(t, l.checkFailures(pm(map[string]property.Value{"gated": str("sometimes")})))
	for _, ok := range []string{"off", "auto", "manual"} {
		require.Empty(t, l.checkFailures(pm(map[string]property.Value{"gated": str(ok)})), ok)
	}

	l.f.failOn = []string{"PUT /settings"}
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: pm(map[string]property.Value{"gated": str("auto")})})
	require.Error(t, err)
	require.Equal(t, "me/g", c.ID, "the repository exists, so it is recorded with its ID")
	require.NotNil(t, l.f.repo("model", "me", "g"))
	require.False(t, hasProp(c.Properties, "gated"))
}
