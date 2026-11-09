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

// Importing an ID that does not exist on the Hub must fail (#1712), while a
// refresh of a resource deleted out of band must still return an empty ID so
// the engine drops it from state.
func TestRead_MissingObject_ImportErrorsRefreshDrops(t *testing.T) {
	cases := []struct {
		typ, id string
		prior   map[string]property.Value
	}{
		{"Repository", "me/doesnotexist", map[string]property.Value{"private": boolean(true)}},
		{"Space", "me/doesnotexist", map[string]property.Value{"sdk": str("gradio")}},
		{"Bucket", "me/doesnotexist", map[string]property.Value{"private": boolean(true)}},
		{"Webhook", "000000000000000000000000", map[string]property.Value{"url": str("https://example.com/hook")}},
		{"Collection", "me/doesnotexist-0123456789abcdef01234567", map[string]property.Value{"title": str("t")}},
		{"CollectionItem", "me/doesnotexist-0123456789abcdef01234567/abc", map[string]property.Value{"collectionSlug": str("me/doesnotexist-0123456789abcdef01234567")}},
		{"InferenceEndpoint", "me/doesnotexist", map[string]property.Value{"modelRepository": str("me/m")}},
		{"RepositoryBranch", "me/doesnotexist@feature", map[string]property.Value{"repoId": str("me/doesnotexist")}},
		{"RepositoryTag", "me/doesnotexist@v1", map[string]property.Value{"repoId": str("me/doesnotexist")}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			l := newLC(t, tc.typ, "x")
			_, err := l.s.Read(pgp.ReadRequest{ID: tc.id, Urn: l.urn, Properties: pm(nil), Inputs: pm(nil)})
			require.Error(t, err, "import of a missing object must fail")
			require.Contains(t, err.Error(), "does not exist")

			prior := pm(tc.prior)
			r, err := l.s.Read(pgp.ReadRequest{ID: tc.id, Urn: l.urn, Properties: prior, Inputs: prior})
			require.NoError(t, err, "refresh of a deleted object must not fail")
			require.Empty(t, r.ID, "refresh of a deleted object returns an empty ID")
		})
	}
}
