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
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func watchedModel(name string) property.Value {
	return obj(map[string]property.Value{"type": str("model"), "name": str(name)})
}

func webhookInputs() property.Map {
	return pm(map[string]property.Value{
		"url":     str("https://example.com/hook"),
		"watched": arr(watchedModel("acme/m")),
		"secret":  str("s3cret"),
	})
}

func TestWebhook_CreatePayloadAndLifecycle(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	in := webhookInputs()
	c := l.create(in)
	require.True(t, l.f.webhook(c.ID) != nil)
	wh := l.f.webhook(c.ID)
	require.Equal(t, "https://example.com/hook", wh.url)
	require.Equal(t, "s3cret", wh.secret)
	require.Equal(t, []string{"repo", "discussion"}, wh.domains, "the Hub default is both domains")
	require.False(t, wh.disabled)

	body := decodeBody(t, l.f.callsMatching("POST", "/api/settings/webhooks")[0].Body)
	require.Equal(t, "https://example.com/hook", body["url"])
	require.NotContains(t, body, "jobSourceId")

	// Server defaults stay out of the inputs; the Hub's value is an output.
	require.False(t, hasProp(c.Properties, "domains"))
	require.ElementsMatch(t, []string{"repo", "discussion"}, strs(c.Properties, "effectiveDomains"))
	// The secret and the token are secret values.
	require.True(t, c.Properties.Get("secret").Secret())

	// Idempotent diff.
	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))

	// Refresh is stable and keeps the write-only secret.
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, c.ID, rd.ID)
	require.Equal(t, "s3cret", rd.Properties.Get("secret").AsString())
	require.True(t, rd.Properties.Get("secret").Secret())
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// Update url, watched list and secret in one call.
	next := with(in, map[string]property.Value{
		"url":     str("https://example.com/hook2"),
		"watched": arr(watchedModel("acme/m"), obj(map[string]property.Value{"type": str("org"), "name": str("acme")})),
		"secret":  str("rotated"),
		"domains": arr(str("repo")),
	})
	d := l.diff(c.ID, rd.Properties, next, in)
	for _, k := range []string{"url", "watched", "secret", "domains"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[k].Kind, k)
	}
	require.False(t, d.DeleteBeforeReplace)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, next, in)
	wh = l.f.webhook(c.ID)
	require.Equal(t, "https://example.com/hook2", wh.url)
	require.Len(t, wh.watched, 2)
	require.Equal(t, "rotated", wh.secret)
	require.Equal(t, []string{"repo"}, wh.domains)
	require.Len(t, l.f.callsMatching("POST", "/api/settings/webhooks/"+c.ID), 1)
	require.ElementsMatch(t, []string{"repo"}, strs(u.Properties, "effectiveDomains"))
	requireNoChanges(t, l.diff(c.ID, u.Properties, next, next))

	// Reordering the watched list is not a change.
	reordered := with(next, map[string]property.Value{"watched": arr(
		obj(map[string]property.Value{"type": str("org"), "name": str("acme")}), watchedModel("acme/m"))})
	requireNoChanges(t, l.diff(c.ID, u.Properties, reordered, next))

	// Unsetting domains means the default (both): that is a change to apply.
	d = l.diff(c.ID, u.Properties, without(next, "domains"), next)
	require.Equal(t, pgp.Update, d.DetailedDiff["domains"].Kind)

	l.del(c.ID, u.Properties)
	require.Nil(t, l.f.webhook(c.ID))
	l.del(c.ID, u.Properties) // already gone: not an error
}

func TestWebhook_DisableAndEnable(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	in := webhookInputs()
	c := l.create(in)

	off := with(in, map[string]property.Value{"disabled": boolean(true)})
	d := l.diff(c.ID, c.Properties, off, in)
	require.Equal(t, pgp.Update, d.DetailedDiff["disabled"].Kind)
	l.f.resetCalls()
	u := l.update(c.ID, c.Properties, off, in)
	require.True(t, l.f.webhook(c.ID).disabled)
	require.Len(t, l.f.callsMatching("POST", "/"+c.ID+"/disable"), 1)
	requireNoChanges(t, l.diff(c.ID, u.Properties, off, off))

	l.update(c.ID, u.Properties, in, off)
	require.False(t, l.f.webhook(c.ID).disabled)
	require.Len(t, l.f.callsMatching("POST", "/"+c.ID+"/enable"), 1)

	// Created disabled.
	m := newLC(t, "Webhook", "hook2")
	mc := m.create(off)
	require.True(t, m.f.webhook(mc.ID).disabled)
	require.True(t, mc.Properties.Get("disabled").AsBool())
}

func TestWebhook_RefreshDetectsDriftAndRepairsIt(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	in := webhookInputs()
	c := l.create(in)
	l.f.mutateWebhook(c.ID, func(w *fakeWebhook) {
		w.url = "https://evil.example/hook"
		w.disabled = true
		w.domains = []string{"repo"}
	})
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, "https://evil.example/hook", getStr(t, rd.Inputs, "url"))
	require.True(t, rd.Inputs.Get("disabled").AsBool())
	require.Equal(t, []string{"repo"}, strs(rd.Inputs, "domains"), "unset domains differ from the default: surfaced")
	d := l.diff(c.ID, rd.Properties, in, rd.Inputs)
	for _, k := range []string{"url", "disabled", "domains"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[k].Kind, k)
	}
	l.update(c.ID, rd.Properties, in, rd.Inputs)
	wh := l.f.webhook(c.ID)
	require.Equal(t, "https://example.com/hook", wh.url)
	require.False(t, wh.disabled)
	require.Equal(t, []string{"repo", "discussion"}, wh.domains)
}

func TestWebhook_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "Webhook", "imp")
	external := &fakeWebhook{
		url: "https://example.com/ext", domains: []string{"repo"}, disabled: true, secret: "hidden",
		watched: []map[string]any{{"type": "user", "name": "me"}, {"type": "dataset", "name": "acme/d"}},
	}
	l.f.addWebhook(external)

	rd := l.read(external.id, empty(), empty())
	require.Equal(t, external.id, rd.ID)
	require.Equal(t, "https://example.com/ext", getStr(t, rd.Inputs, "url"))
	require.Equal(t, []string{"repo"}, strs(rd.Inputs, "domains"))
	require.True(t, rd.Inputs.Get("disabled").AsBool())
	require.Len(t, rd.Inputs.Get("watched").AsArray().AsSlice(), 2)
	require.False(t, hasProp(rd.Inputs, "secret"), "the secret is write-only and cannot be imported")

	in := l.checked(rd.Inputs)
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, in, rd.Inputs))
}

func TestWebhook_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "Webhook", "gone")
	in := webhookInputs()
	c := l.create(in)
	l.f.deleteWebhookOutOfBand(c.ID)
	require.Empty(t, l.read(c.ID, c.Properties, in).ID)
	l.del(c.ID, c.Properties)
}

func TestWebhook_Check(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	bad := []property.Map{
		with(webhookInputs(), map[string]property.Value{"url": str("")}),
		with(webhookInputs(), map[string]property.Value{"watched": arr()}),
		with(webhookInputs(), map[string]property.Value{"watched": arr(obj(map[string]property.Value{"type": str("team"), "name": str("x")}))}),
		with(webhookInputs(), map[string]property.Value{"watched": arr(obj(map[string]property.Value{"type": str("model"), "name": str("")}))}),
		with(webhookInputs(), map[string]property.Value{"domains": arr(str("repo"), str("job"))}),
	}
	for i, in := range bad {
		require.NotEmpty(t, l.checkFailures(in), "case %d", i)
	}
	require.Empty(t, l.checkFailures(webhookInputs()))
}

func TestWebhook_FailureAfterCreateKeepsTheWebhook(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	l.f.failOn = []string{"POST /disable"}
	in := with(webhookInputs(), map[string]property.Value{"disabled": boolean(true)})
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.checked(in)})
	require.Error(t, err)
	// The webhook exists on the Hub and is recorded with its ID and partial state.
	require.NotEmpty(t, c.ID)
	require.NotNil(t, l.f.webhook(c.ID))
	require.False(t, hasProp(c.Properties, "disabled"))
}

func TestWebhook_Lookup(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	c := l.create(webhookInputs())
	out, err := invoke(t, l.s, "getWebhook", map[string]property.Value{"webhookId": str(c.ID)})
	require.NoError(t, err)
	require.Equal(t, c.ID, getStr(t, out, "webhookId"))
	require.Equal(t, "https://example.com/hook", getStr(t, out, "url"))
	require.Len(t, out.Get("watched").AsArray().AsSlice(), 1)
	require.False(t, hasProp(out, "secret"))

	_, err = invoke(t, l.s, "getWebhook", map[string]property.Value{"webhookId": str("nope")})
	require.ErrorContains(t, err, `webhook "nope" was not found`)
}

// #1590: a preview Create returned the resource name as the ID, so a program passing webhook.id to
// an invoke (getWebhook) got a concrete, wrong ID during `pulumi preview`, ran the invoke, and
// failed. The real ID is assigned by the Hub, so a preview has none and `id` stays unknown.
func TestWebhook_PreviewHasNoIDSoLookupsDefer(t *testing.T) {
	l := newLC(t, "Webhook", "hook")
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.checked(webhookInputs()), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, c.ID, "a preview has no real ID, so the id output stays unknown")
	require.Empty(t, l.f.callsMatching("POST", "/api/settings/webhooks"), "a preview does not create the webhook")
}
