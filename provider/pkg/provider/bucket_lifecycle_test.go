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

func TestBucket_Lifecycle(t *testing.T) {
	l := newLC(t, "Bucket", "artifacts")
	in := pm(map[string]property.Value{"private": boolean(true), "region": str("eu"), "resourceGroupId": str("rg0")})
	c := l.create(in)
	require.Equal(t, "me/artifacts", c.ID)
	require.Equal(t, "me/artifacts", getStr(t, c.Properties, "bucketId"))
	require.Equal(t, "https://huggingface.co/buckets/me/artifacts", getStr(t, c.Properties, "url"))
	b := l.f.bucket("me/artifacts")
	require.True(t, b.private)
	require.Equal(t, "eu", b.region)
	body := decodeBody(t, l.f.callsMatching("POST", "/api/buckets/me/artifacts")[0].Body)
	require.Equal(t, map[string]any{"visibility": "private", "region": "eu", "resourceGroupId": "rg0"}, body)

	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, c.ID, rd.ID)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// The documented API cannot update a bucket: everything but the token replaces it.
	for k, v := range map[string]property.Value{
		"private": boolean(false), "region": str("us"), "resourceGroupId": str("rg1"),
		"namespace": str("acme"), "name": str("other"),
	} {
		d := l.diff(c.ID, rd.Properties, with(in, map[string]property.Value{k: v}), in)
		require.Equal(t, pgp.UpdateReplace, d.DetailedDiff[k].Kind, k)
		require.True(t, d.DeleteBeforeReplace, k)
	}
	// An explicit name equal to the ID's name is no change.
	requireNoChanges(t, l.diff(c.ID, rd.Properties, with(in, map[string]property.Value{"name": str("artifacts")}), in))

	tok := with(in, map[string]property.Value{"token": str(fakeToken)})
	require.Equal(t, pgp.Update, l.diff(c.ID, rd.Properties, tok, in).DetailedDiff["token"].Kind)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, tok, in)
	require.Empty(t, l.f.calls)
	require.Equal(t, c.ID, getStr(t, u.Properties, "bucketId"))

	l.del(c.ID, u.Properties)
	require.Nil(t, l.f.bucket("me/artifacts"))
	l.del(c.ID, u.Properties)
}

func TestBucket_ExplicitNameAndNamespace(t *testing.T) {
	l := newLC(t, "Bucket", "logical")
	in := pm(map[string]property.Value{"name": str("physical"), "namespace": str("acme")})
	c := l.create(in)
	require.Equal(t, "acme/physical", c.ID)
	require.NotNil(t, l.f.bucket("acme/physical"))
	require.Equal(t, "public", decodeBody(t, l.f.callsMatching("POST", "/api/buckets/acme/physical")[0].Body)["visibility"])
}

func TestBucket_RefreshDriftAndImport(t *testing.T) {
	l := newLC(t, "Bucket", "b")
	in := pm(map[string]property.Value{})
	c := l.create(in)
	l.f.mutateBucket("me/b", func(b *fakeBucket) { b.private = true })
	rd := l.read(c.ID, c.Properties, in)
	require.True(t, rd.Properties.Get("private").AsBool())
	require.True(t, rd.Inputs.Get("private").AsBool())
	d := l.diff(c.ID, rd.Properties, in, rd.Inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["private"].Kind, "visibility cannot be changed in place")

	l.f.addBucket("acme/ext", true)
	imp := l.read("acme/ext", empty(), empty())
	require.Equal(t, "acme/ext", imp.ID)
	require.Equal(t, "acme", getStr(t, imp.Inputs, "namespace"))
	require.Equal(t, "ext", getStr(t, imp.Inputs, "name"))
	require.True(t, imp.Inputs.Get("private").AsBool())
	require.Equal(t, "https://huggingface.co/buckets/acme/ext", getStr(t, imp.Properties, "url"))
	chk := l.checked(imp.Inputs)
	requireNoChanges(t, l.diff(imp.ID, imp.Properties, chk, imp.Inputs))
}

func TestBucket_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "Bucket", "gone")
	in := pm(map[string]property.Value{})
	c := l.create(in)
	l.f.deleteBucketOutOfBand("me/gone")
	require.Empty(t, l.read(c.ID, c.Properties, in).ID)
	l.del(c.ID, c.Properties)
}

func TestBucket_PreviewAndDuplicate(t *testing.T) {
	l := newLC(t, "Bucket", "b")
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: pm(nil), DryRun: true})
	require.NoError(t, err)
	require.NotEmpty(t, c.ID)
	require.Empty(t, l.f.calls)
	l.create(pm(nil))
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: pm(nil)})
	require.Error(t, err, "409 for an existing bucket")
}

func TestBucket_Lookup(t *testing.T) {
	l := newLC(t, "Bucket", "b")
	l.f.addBucket("me/data", true)
	out, err := invoke(t, l.s, "getBucket", map[string]property.Value{"name": str("data")})
	require.NoError(t, err)
	require.Equal(t, "me/data", getStr(t, out, "bucketId"))
	require.True(t, out.Get("private").AsBool())
	require.Equal(t, "https://huggingface.co/buckets/me/data", getStr(t, out, "url"))

	_, err = invoke(t, l.s, "getBucket", map[string]property.Value{"namespace": str("acme"), "name": str("zz")})
	require.ErrorContains(t, err, `bucket "acme/zz" was not found`)
}
