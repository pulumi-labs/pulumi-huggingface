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

// #1550: an update preview that changes only a mutable input keeps the
// identifier outputs (fixed at creation) known, instead of "known after apply".
func TestWiring_UpdatePreviewKeepsIdentifiersKnown(t *testing.T) {
	type tc struct {
		typ    string
		inputs func(l *lc) property.Map
		change map[string]property.Value
		known  []string
	}
	cases := []tc{
		{"Collection", func(*lc) property.Map { return collectionInputs() }, map[string]property.Value{"description": str("new text")}, []string{"slug", "url", "effectiveTheme"}},
		{"Collection", func(*lc) property.Map { return collectionInputs() }, map[string]property.Value{"theme": str("pink")}, []string{"slug", "url"}},
		{"Repository", func(*lc) property.Map { return pm(map[string]property.Value{}) }, map[string]property.Value{"private": boolean(true)}, []string{"repoId", "url"}},
		{"Space", func(*lc) property.Map { return spaceInputs() }, map[string]property.Value{"private": boolean(true)}, []string{"spaceId", "url"}},
		{"Bucket", func(*lc) property.Map { return pm(map[string]property.Value{}) }, map[string]property.Value{"token": str(fakeToken)}, []string{"bucketId", "url"}},
		{"CollectionItem", func(l *lc) property.Map { return itemInputs(l.f.addCollection("me", "Models")) }, map[string]property.Value{"note": str("changed")}, []string{"itemObjectId"}},
		{"Webhook", func(*lc) property.Map { return webhookInputs() }, map[string]property.Value{"disabled": boolean(true)}, []string{"effectiveDomains"}},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			l := newLC(t, c.typ, "r")
			in := c.inputs(l)
			created := l.create(in)
			changed := with(in, c.change)
			u, err := l.s.Update(pgp.UpdateRequest{ID: created.ID, Urn: l.urn, State: created.Properties, Inputs: changed, OldInputs: in, DryRun: true})
			require.NoError(t, err)
			for _, k := range c.known {
				v, ok := u.Properties.GetOk(k)
				require.True(t, ok, k)
				require.False(t, v.HasComputed(), "%s must stay known", k)
				require.Equal(t, created.Properties.Get(k), v, k)
			}
		})
	}
}
