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

	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// #1551: a Check input that is not known yet (another resource's output) is
// decoded to the zero value by infer.DefaultCheck and must not be rejected at
// preview; a literal malformed value still is.
func TestCheck_UnknownInputsAreAccepted(t *testing.T) {
	computed := property.New(property.Computed)
	cases := []struct {
		typ    string
		inputs map[string]property.Value
		prop   string
		bad    property.Value
	}{
		{"Space", map[string]property.Value{"sdk": str("gradio")}, "sdk", str("nope")},
		{"Webhook", map[string]property.Value{"url": str("https://x.example/h"), "watched": arr(obj(map[string]property.Value{"type": str("model"), "name": str("me/m")}))}, "url", str("")},
		{"Webhook", map[string]property.Value{"url": str("https://x.example/h"), "watched": arr(obj(map[string]property.Value{"type": str("model"), "name": str("me/m")}))}, "watched", arr()},
		{"RepositoryBranch", map[string]property.Value{"repoId": str("me/m"), "branch": str("dev")}, "repoId", str("bad")},
		{"RepositoryBranch", map[string]property.Value{"repoId": str("me/m"), "branch": str("dev")}, "branch", str("")},
		{"RepositoryTag", map[string]property.Value{"repoId": str("me/m"), "tag": str("v1")}, "repoId", str("bad")},
		{"RepositoryTag", map[string]property.Value{"repoId": str("me/m"), "tag": str("v1")}, "tag", str("")},
		{"Collection", map[string]property.Value{"title": str("t")}, "title", str(" ")},
		{"CollectionItem", map[string]property.Value{"itemType": str("model"), "collectionSlug": str("me/c-1"), "itemId": str("me/m")}, "itemType", str("nope")},
		{"CollectionItem", map[string]property.Value{"itemType": str("model"), "collectionSlug": str("me/c-1"), "itemId": str("me/m")}, "collectionSlug", str("bad")},
		{"CollectionItem", map[string]property.Value{"itemType": str("model"), "collectionSlug": str("me/c-1"), "itemId": str("me/m")}, "itemId", str("")},
	}
	for _, c := range cases {
		t.Run(c.typ+"/"+c.prop, func(t *testing.T) {
			l := newLC(t, c.typ, "x")
			require.Empty(t, l.checkFailures(pm(c.inputs)), "valid inputs")
			bad := map[string]property.Value{}
			for k, v := range c.inputs {
				bad[k] = v
			}
			bad[c.prop] = c.bad
			require.NotEmpty(t, l.checkFailures(pm(bad)), "literal malformed %s", c.prop)
			unk := map[string]property.Value{}
			for k, v := range c.inputs {
				unk[k] = v
			}
			unk[c.prop] = computed
			require.Empty(t, l.checkFailures(pm(unk)), "unknown %s", c.prop)
		})
	}
}

// #1976: a literal url that is not an absolute http(s) URL fails at Check (the Hub only
// answers 400 at create); an unknown url still passes preview.
func TestWebhookCheck_RejectsMalformedURL(t *testing.T) {
	watched := arr(obj(map[string]property.Value{"type": str("model"), "name": str("me/m")}))
	l := newLC(t, "Webhook", "x")
	for _, bad := range []string{"not-a-url", "example.com/hook", "ftp://example.com/h", "https://", "http:///path", "://x"} {
		fails := l.checkFailures(pm(map[string]property.Value{"url": str(bad), "watched": watched}))
		require.NotEmpty(t, fails, bad)
	}
	for _, ok := range []string{"https://example.com/hook", "http://localhost:8080/h?x=1"} {
		require.Empty(t, l.checkFailures(pm(map[string]property.Value{"url": str(ok), "watched": watched})), ok)
	}
	require.Empty(t, l.checkFailures(pm(map[string]property.Value{"url": property.New(property.Computed), "watched": watched})))
}
