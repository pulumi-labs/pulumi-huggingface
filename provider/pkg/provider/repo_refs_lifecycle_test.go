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

func branchInputs() property.Map {
	return pm(map[string]property.Value{"repoId": str("acme/m"), "branch": str("dev")})
}

func TestRepositoryBranch_Lifecycle(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "br")
	l.f.addRepo("model", "acme", "m", nil)
	main, _ := l.f.refs("model", "acme", "m")

	in := with(branchInputs(), map[string]property.Value{"startingPoint": str("main")})
	c := l.create(in)
	require.Equal(t, "models/acme/m@dev", c.ID)
	branches, _ := l.f.refs("model", "acme", "m")
	require.Equal(t, main["main"], branches["dev"])
	require.Equal(t, main["main"], getStr(t, c.Properties, "targetCommit"))
	body := decodeBody(t, l.f.callsMatching("POST", "/api/models/acme/m/branch/dev")[0].Body)
	require.Equal(t, map[string]any{"startingPoint": "main"}, body)

	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
	// Without a starting point the default applies, after an import too.
	requireNoChanges(t, l.diff(c.ID, c.Properties, branchInputs(), in))
	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// A moved branch only changes the output.
	l.f.mutateRefs("model", "acme", "m", func(b, _ map[string]string) { b["dev"] = "f00d" })
	rd = l.read(c.ID, c.Properties, in)
	require.Equal(t, "f00d", getStr(t, rd.Properties, "targetCommit"))
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	// Everything is immutable: changes replace.
	for k, v := range map[string]property.Value{"repoId": str("acme/n"), "branch": str("other"), "repoType": str("dataset"), "startingPoint": str("v1")} {
		d := l.diff(c.ID, rd.Properties, with(in, map[string]property.Value{k: v}), in)
		require.Equal(t, pgp.UpdateReplace, d.DetailedDiff[k].Kind, k)
		require.True(t, d.DeleteBeforeReplace, k)
	}
	// A token change is a plain update that touches nothing on the Hub.
	tok := with(in, map[string]property.Value{"token": str(fakeToken)})
	d := l.diff(c.ID, rd.Properties, tok, in)
	require.Equal(t, pgp.Update, d.DetailedDiff["token"].Kind)
	l.f.resetCalls()
	l.update(c.ID, rd.Properties, tok, in)
	require.Empty(t, l.f.calls)

	l.del(c.ID, rd.Properties)
	branches, _ = l.f.refs("model", "acme", "m")
	require.NotContains(t, branches, "dev")
	l.del(c.ID, rd.Properties)
}

func TestRepositoryBranch_SlashedNameAndDatasets(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "br")
	l.f.addRepo("dataset", "acme", "d", nil)
	in := pm(map[string]property.Value{"repoId": str("acme/d"), "repoType": str("dataset"), "branch": str("feature/x")})
	c := l.create(in)
	require.Equal(t, "datasets/acme/d@feature/x", c.ID)
	branches, _ := l.f.refs("dataset", "acme", "d")
	require.Contains(t, branches, "feature/x")

	rd := l.read(c.ID, c.Properties, in)
	require.Equal(t, "feature/x", getStr(t, rd.Inputs, "branch"))
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))
	l.del(c.ID, rd.Properties)
	branches, _ = l.f.refs("dataset", "acme", "d")
	require.NotContains(t, branches, "feature/x")
}

func TestRepositoryBranch_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "imp")
	l.f.addRepo("model", "acme", "m", nil)
	l.f.addRepo("space", "acme", "s", nil)
	l.f.mutateRefs("model", "acme", "m", func(b, _ map[string]string) { b["release"] = "abc" })
	l.f.mutateRefs("space", "acme", "s", func(b, _ map[string]string) { b["release"] = "def" })

	for _, tc := range []struct{ id, typ, repo string }{
		{"models/acme/m@release", "model", "acme/m"},
		{"acme/m@release", "model", "acme/m"},
		{"spaces/acme/s@release", "space", "acme/s"},
	} {
		rd := l.read(tc.id, empty(), empty())
		require.Equal(t, tc.id, rd.ID, tc.id)
		require.Equal(t, tc.repo, getStr(t, rd.Inputs, "repoId"))
		require.Equal(t, tc.typ, getStr(t, rd.Inputs, "repoType"))
		require.Equal(t, "release", getStr(t, rd.Inputs, "branch"))
		in := l.checked(rd.Inputs)
		requireNoChanges(t, l.diff(rd.ID, rd.Properties, in, rd.Inputs))
	}
	// A user's starting point added after an import never replaces the branch.
	rd := l.read("models/acme/m@release", empty(), empty())
	user := with(rd.Inputs, map[string]property.Value{"startingPoint": str("main")})
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, user, rd.Inputs))

	for _, bad := range []string{"acme/m", "@x", "acme/m@", "justone@x", "a/b/c@x"} {
		_, err := l.s.Read(pgp.ReadRequest{ID: bad, Urn: l.urn, Properties: empty(), Inputs: empty()})
		require.Error(t, err, bad)
	}
}

func TestRepositoryBranch_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "gone")
	l.f.addRepo("model", "acme", "m", nil)
	in := branchInputs()
	c := l.create(in)
	l.f.mutateRefs("model", "acme", "m", func(b, _ map[string]string) { delete(b, "dev") })
	require.Empty(t, l.read(c.ID, c.Properties, in).ID, "branch deleted")
	l.del(c.ID, c.Properties)

	c = l.create(in)
	l.f.deleteRepoOutOfBand("model", "acme", "m")
	require.Empty(t, l.read(c.ID, c.Properties, in).ID, "repository deleted")
	l.del(c.ID, c.Properties)
}

func TestRepositoryBranch_CheckAndErrors(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "br")
	require.NotEmpty(t, l.checkFailures(with(branchInputs(), map[string]property.Value{"repoId": str("nonamespace")})))
	require.NotEmpty(t, l.checkFailures(with(branchInputs(), map[string]property.Value{"repoType": str("bucket")})))
	require.NotEmpty(t, l.checkFailures(with(branchInputs(), map[string]property.Value{"branch": str("")})))

	l.f.addRepo("model", "acme", "m", nil)
	l.create(branchInputs())
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: branchInputs()})
	require.Error(t, err, "409 for an existing branch")
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: with(pm(nil), map[string]property.Value{
		"repoId": str("acme/m"), "branch": str("x"), "startingPoint": str("does-not-exist")})})
	require.Error(t, err, "unknown starting point")
}

func TestRepositoryBranch_Lookup(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "br")
	l.f.addRepo("model", "acme", "m", nil)
	main, _ := l.f.refs("model", "acme", "m")
	out, err := invoke(t, l.s, "getRepositoryBranch", map[string]property.Value{"repoId": str("acme/m"), "branch": str("main")})
	require.NoError(t, err)
	require.Equal(t, main["main"], getStr(t, out, "targetCommit"))
	require.Equal(t, "refs/heads/main", getStr(t, out, "ref"))
	require.Equal(t, "model", getStr(t, out, "repoType"))

	_, err = invoke(t, l.s, "getRepositoryBranch", map[string]property.Value{"repoId": str("acme/m"), "branch": str("zz")})
	require.ErrorContains(t, err, `branch "zz" was not found in "acme/m"`)
	_, err = invoke(t, l.s, "getRepositoryBranch", map[string]property.Value{"repoId": str("acme/none"), "branch": str("main")})
	require.ErrorContains(t, err, `repository "acme/none" was not found`)
}

// ---------------------------------------------------------------------------
// RepositoryTag
// ---------------------------------------------------------------------------

func tagInputs() property.Map {
	return pm(map[string]property.Value{"repoId": str("acme/m"), "tag": str("v1.0"), "message": str("first release")})
}

func TestRepositoryTag_Lifecycle(t *testing.T) {
	l := newLC(t, "RepositoryTag", "tag")
	l.f.addRepo("model", "acme", "m", nil)
	branches, _ := l.f.refs("model", "acme", "m")
	in := with(tagInputs(), map[string]property.Value{"revision": str("main")})
	c := l.create(in)
	require.Equal(t, "models/acme/m@v1.0", c.ID)
	_, tags := l.f.refs("model", "acme", "m")
	require.Equal(t, branches["main"], tags["v1.0"], "tags the default revision")
	require.Equal(t, branches["main"], getStr(t, c.Properties, "targetCommit"))
	posts := l.f.callsMatching("POST", "/api/models/acme/m/tag/main")
	require.Len(t, posts, 1)
	require.Equal(t, map[string]any{"tag": "v1.0", "message": "first release"}, decodeBody(t, posts[0].Body))

	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))
	requireNoChanges(t, l.diff(c.ID, c.Properties, tagInputs(), in)) // the default revision
	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))

	for k, v := range map[string]property.Value{"tag": str("v2"), "repoId": str("acme/n"), "revision": str("dev"), "message": str("other")} {
		d := l.diff(c.ID, rd.Properties, with(in, map[string]property.Value{k: v}), in)
		require.Equal(t, pgp.UpdateReplace, d.DetailedDiff[k].Kind, k)
		require.True(t, d.DeleteBeforeReplace, k)
	}

	l.del(c.ID, rd.Properties)
	_, tags = l.f.refs("model", "acme", "m")
	require.NotContains(t, tags, "v1.0")
	l.del(c.ID, rd.Properties)
}

func TestRepositoryTag_RevisionAndDatasets(t *testing.T) {
	l := newLC(t, "RepositoryTag", "tag")
	l.f.addRepo("dataset", "acme", "d", nil)
	l.f.mutateRefs("dataset", "acme", "d", func(b, _ map[string]string) { b["dev"] = "dev-commit" })
	in := pm(map[string]property.Value{"repoId": str("acme/d"), "repoType": str("dataset"), "tag": str("v0"), "revision": str("dev")})
	c := l.create(in)
	require.Equal(t, "datasets/acme/d@v0", c.ID)
	_, tags := l.f.refs("dataset", "acme", "d")
	require.Equal(t, "dev-commit", tags["v0"])
	require.Len(t, l.f.callsMatching("POST", "/api/datasets/acme/d/tag/dev"), 1)
}

func TestRepositoryTag_ImportDeletedAndLookup(t *testing.T) {
	l := newLC(t, "RepositoryTag", "imp")
	l.f.addRepo("model", "acme", "m", nil)
	l.f.mutateRefs("model", "acme", "m", func(_, tags map[string]string) { tags["v9"] = "c9" })

	rd := l.read("models/acme/m@v9", empty(), empty())
	require.Equal(t, "acme/m", getStr(t, rd.Inputs, "repoId"))
	require.Equal(t, "v9", getStr(t, rd.Inputs, "tag"))
	require.Equal(t, "c9", getStr(t, rd.Properties, "targetCommit"))
	in := l.checked(rd.Inputs)
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, in, rd.Inputs))
	// Revision and message are create-only: adding them after an import is no change.
	user := with(rd.Inputs, map[string]property.Value{"revision": str("main"), "message": str("m")})
	requireNoChanges(t, l.diff(rd.ID, rd.Properties, user, rd.Inputs))

	out, err := invoke(t, l.s, "getRepositoryTag", map[string]property.Value{"repoId": str("acme/m"), "tag": str("v9")})
	require.NoError(t, err)
	require.Equal(t, "refs/tags/v9", getStr(t, out, "ref"))
	require.Equal(t, "c9", getStr(t, out, "targetCommit"))
	_, err = invoke(t, l.s, "getRepositoryTag", map[string]property.Value{"repoId": str("acme/m"), "tag": str("nope")})
	require.ErrorContains(t, err, `tag "nope" was not found in "acme/m"`)

	l.f.mutateRefs("model", "acme", "m", func(_, tags map[string]string) { delete(tags, "v9") })
	require.Empty(t, l.read("models/acme/m@v9", empty(), empty()).ID)
	l.f.deleteRepoOutOfBand("model", "acme", "m")
	require.Empty(t, l.read("models/acme/m@v9", empty(), empty()).ID)
	l.del("models/acme/m@v9", rd.Properties)
}

func TestRepositoryTag_CheckAndConflict(t *testing.T) {
	l := newLC(t, "RepositoryTag", "tag")
	require.NotEmpty(t, l.checkFailures(with(tagInputs(), map[string]property.Value{"tag": str("")})))
	require.NotEmpty(t, l.checkFailures(with(tagInputs(), map[string]property.Value{"repoId": str("x")})))
	l.f.addRepo("model", "acme", "m", nil)
	l.create(tagInputs())
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: tagInputs()})
	require.Error(t, err, "409 for an existing tag")
}

func TestRepositoryRefs_FailureAfterCreateKeepsTheRef(t *testing.T) {
	l := newLC(t, "RepositoryBranch", "br")
	l.f.addRepo("model", "acme", "m", nil)
	l.f.failOn = []string{"GET /refs"}
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: branchInputs()})
	require.Error(t, err)
	require.Equal(t, "models/acme/m@dev", c.ID, "the branch exists, so it is recorded with its ID")
	branches, _ := l.f.refs("model", "acme", "m")
	require.Contains(t, branches, "dev")
}

// An annotated tag (created with a message) is reported by /refs as the tag
// object, not the commit. targetCommit must always be the commit (#1675).
func TestRepositoryTag_TargetCommitIsPeeledForAnnotatedTags(t *testing.T) {
	l := newLC(t, "RepositoryTag", "tag")
	l.f.addRepo("model", "acme", "m", nil)
	branches, _ := l.f.refs("model", "acme", "m")
	main := branches["main"]

	annotated := with(tagInputs(), map[string]property.Value{"revision": str("main")})
	c := l.create(annotated)
	require.Equal(t, main, getStr(t, c.Properties, "targetCommit"), "create reports the commit")

	// /refs itself reports a tag object, so the peel is what makes this equal.
	l.f.mu.Lock()
	listed := l.f.repos[repoKey("model", "acme", "m")].tagsAsListed()["v1.0"]
	l.f.mu.Unlock()
	require.NotEqual(t, main, listed, "the fake Hub reports the tag object in /refs")

	rd := l.read(c.ID, c.Properties, annotated)
	require.Equal(t, main, getStr(t, rd.Properties, "targetCommit"), "refresh reports the commit")
	imp := l.read(c.ID, empty(), empty())
	require.Equal(t, main, getStr(t, imp.Properties, "targetCommit"), "import reports the commit")

	out, err := invoke(t, l.s, "getRepositoryTag", map[string]property.Value{"repoId": str("acme/m"), "tag": str("v1.0")})
	require.NoError(t, err)
	require.Equal(t, main, getStr(t, out, "targetCommit"), "the lookup reports the commit")

	// A lightweight tag (no message) is the commit already.
	plain := pm(map[string]property.Value{"repoId": str("acme/m"), "tag": str("plain")})
	pc := l.create(plain)
	require.Equal(t, main, getStr(t, pc.Properties, "targetCommit"))
	out, err = invoke(t, l.s, "getRepositoryTag", map[string]property.Value{"repoId": str("acme/m"), "tag": str("plain")})
	require.NoError(t, err)
	require.Equal(t, main, getStr(t, out, "targetCommit"))
}
