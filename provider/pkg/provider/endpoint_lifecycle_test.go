package provider

import (
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func endpointInputs() property.Map {
	return pm(map[string]property.Value{
		"modelRepository": str("org/model"),
		"modelFramework":  str("pytorch"),
		"modelTask":       str("text-classification"),
		"vendor":          str("aws"),
		"region":          str("us-east-1"),
		"accelerator":     str("cpu"),
		"instanceType":    str("intel-icl"),
		"instanceSize":    str("x2"),
	})
}

func TestEndpoint_CreatePayloadMatchesTheAPI(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	l.f.pollsUntilRunning = 3

	c := l.create(endpointInputs())
	require.Equal(t, "me/ep", c.ID)
	require.Equal(t, "me", getStr(t, c.Properties, "namespace"))

	// create waits for the endpoint, so outputs are populated on return.
	require.Equal(t, "running", getStr(t, c.Properties, "status"))
	require.Equal(t, "https://ep.endpoints.huggingface.cloud", getStr(t, c.Properties, "url"))
	require.GreaterOrEqual(t, len(l.f.callsMatching("GET", "/v2/endpoint/me/ep")), 3)

	posts := l.f.callsMatching("POST", "/v2/endpoint/me")
	require.Len(t, posts, 1)
	body := decodeBody(t, posts[0].Body)
	// The legacy "protected" type is gone: the default is "authenticated".
	require.Equal(t, "authenticated", body["type"])
	// model.image is required; the stock image is {"huggingface": {}}.
	require.Equal(t, map[string]any{"huggingface": map[string]any{}}, dig(body, "model", "image"))
	require.Equal(t, "org/model", dig(body, "model", "repository"))
	require.Equal(t, "aws", dig(body, "provider", "vendor"))
	require.Equal(t, float64(1), dig(body, "compute", "scaling", "minReplica"))
	require.NotContains(t, body, "accountId")
	require.NotContains(t, body, "privateService")
	require.NotContains(t, body, "env")
}

func TestEndpoint_CustomImagePayload(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	in := with(endpointInputs(), map[string]property.Value{
		"customImageUrl":         str("ghcr.io/huggingface/text-generation-inference:2.4.0"),
		"customImagePort":        num(8080),
		"customImageHealthRoute": str("/health"),
		"customImageEnvVars":     strMap(map[string]string{"MAX_TOKENS": "512"}),
		"endpointType":           str("public"),
	})
	l.create(in)

	stored := l.f.endpoint("me/ep")
	// healthRoute is flat on the custom container, env is model.env.
	require.Equal(t, "/health", dig(stored, "model", "image", "custom", "healthRoute"))
	require.Equal(t, float64(8080), dig(stored, "model", "image", "custom", "port"))
	require.Equal(t, "ghcr.io/huggingface/text-generation-inference:2.4.0", dig(stored, "model", "image", "custom", "url"))
	require.Equal(t, map[string]any{"MAX_TOKENS": "512"}, dig(stored, "model", "env"))
	require.Nil(t, dig(stored, "model", "image", "custom", "env"))
	require.Nil(t, dig(stored, "model", "image", "custom", "health"))
	require.Equal(t, "public", stored["type"])

	body := decodeBody(t, l.f.callsMatching("POST", "/v2/endpoint/me")[0].Body)
	require.NotContains(t, dig(body, "model", "image", "custom"), "env")
}

func TestEndpoint_PrivateServicePayload(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	in := with(endpointInputs(), map[string]property.Value{
		"endpointType": str("private"),
		"accountId":    str("123456789012"),
	})
	c := l.create(in)

	body := decodeBody(t, l.f.callsMatching("POST", "/v2/endpoint/me")[0].Body)
	// accountId lives in privateService, never at the top level.
	require.NotContains(t, body, "accountId")
	require.Equal(t, "123456789012", dig(body, "privateService", "accountId"))
	require.Equal(t, "us-east-1", dig(body, "privateService", "region"), "defaults to the endpoint region")

	// A refresh keeps the inputs stable and the diff clean.
	r := l.read(c.ID, c.Properties, in)
	require.Equal(t, "123456789012", getStr(t, r.Inputs, "accountId"))
	requireNoChanges(t, l.diff(c.ID, r.Properties, in, in))

	// The private link account cannot be changed in place.
	changed := with(in, map[string]property.Value{"accountId": str("210987654321")})
	d := l.diff(c.ID, c.Properties, changed, in)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["accountId"].Kind)
	require.True(t, d.DeleteBeforeReplace)

	// An explicit link region is sent and replaces on change.
	in2 := with(in, map[string]property.Value{"privateLinkRegion": str("eu-west-1")})
	d = l.diff(c.ID, c.Properties, in2, in)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["privateLinkRegion"].Kind)
}

func TestEndpoint_Lifecycle(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	inputs := endpointInputs()
	c := l.create(inputs)
	state := c.Properties

	// Idempotent diff: the same inputs, with or without explicit defaults,
	// report no changes against the state create returned.
	for _, in := range []property.Map{
		inputs,
		with(inputs, map[string]property.Value{"endpointType": str("authenticated"), "minReplica": num(1), "maxReplica": num(1)}),
		with(inputs, map[string]property.Value{"namespace": str("me")}),
	} {
		requireNoChanges(t, l.diff(c.ID, state, in, inputs))
	}

	// Refresh: no drift, inputs unchanged, namespace is not blanked.
	r := l.read(c.ID, state, inputs)
	require.Equal(t, c.ID, r.ID)
	require.Equal(t, "me", getStr(t, r.Properties, "namespace"))
	require.Equal(t, "running", getStr(t, r.Properties, "status"))
	require.Equal(t, "org/model", getStr(t, r.Inputs, "modelRepository"))
	require.False(t, hasProp(r.Inputs, "namespace"), "unset inputs stay unset on refresh")
	require.False(t, hasProp(r.Inputs, "endpointType"))
	requireNoChanges(t, l.diff(c.ID, r.Properties, inputs, inputs))

	// Every updatable property reaches the API.
	updated := with(inputs, map[string]property.Value{
		"modelRepository":        str("org/other-model"),
		"modelRevision":          str("abc123"),
		"modelFramework":         str("custom"),
		"modelTask":              str("feature-extraction"),
		"customImageUrl":         str("ghcr.io/huggingface/text-embeddings-inference:1.5"),
		"customImagePort":        num(8081),
		"customImageHealthRoute": str("/healthz"),
		"customImageEnvVars":     strMap(map[string]string{"A": "1"}),
		"vendor":                 str("azure"),
		"region":                 str("eastus"),
		"accelerator":            str("gpu"),
		"instanceType":           str("nvidia-a10g"),
		"instanceSize":           str("x4"),
		"minReplica":             num(0),
		"maxReplica":             num(3),
		"scaleToZeroTimeout":     num(30),
		"endpointType":           str("public"),
	})
	d := l.diff(c.ID, state, updated, inputs)
	require.True(t, d.HasChanges)
	for _, prop := range []string{
		"modelRepository", "modelRevision", "modelFramework", "modelTask", "customImageUrl", "customImagePort",
		"customImageHealthRoute", "customImageEnvVars", "vendor", "region", "accelerator", "instanceType",
		"instanceSize", "minReplica", "maxReplica", "scaleToZeroTimeout", "endpointType",
	} {
		require.Equal(t, pgp.Update, d.DetailedDiff[prop].Kind, prop)
	}
	require.False(t, d.DeleteBeforeReplace)

	l.f.resetCalls()
	u := l.update(c.ID, state, updated, inputs)
	require.Equal(t, "running", getStr(t, u.Properties, "status"))
	require.Equal(t, "https://ep.endpoints.huggingface.cloud", getStr(t, u.Properties, "url"))
	puts := l.f.callsMatching("PUT", "/v2/endpoint/me/ep")
	require.Len(t, puts, 1)
	stored := l.f.endpoint("me/ep")
	require.Equal(t, "public", stored["type"])
	require.Equal(t, "org/other-model", dig(stored, "model", "repository"))
	require.Equal(t, "abc123", dig(stored, "model", "revision"))
	require.Equal(t, "custom", dig(stored, "model", "framework"))
	require.Equal(t, "feature-extraction", dig(stored, "model", "task"))
	require.Equal(t, "ghcr.io/huggingface/text-embeddings-inference:1.5", dig(stored, "model", "image", "custom", "url"))
	require.Equal(t, float64(8081), dig(stored, "model", "image", "custom", "port"))
	require.Equal(t, "/healthz", dig(stored, "model", "image", "custom", "healthRoute"))
	require.Equal(t, map[string]any{"A": "1"}, dig(stored, "model", "env"))
	require.Equal(t, "azure", dig(stored, "provider", "vendor"))
	require.Equal(t, "eastus", dig(stored, "provider", "region"))
	require.Equal(t, "gpu", dig(stored, "compute", "accelerator"))
	require.Equal(t, "nvidia-a10g", dig(stored, "compute", "instanceType"))
	require.Equal(t, "x4", dig(stored, "compute", "instanceSize"))
	require.Equal(t, float64(0), dig(stored, "compute", "scaling", "minReplica"))
	require.Equal(t, float64(3), dig(stored, "compute", "scaling", "maxReplica"))
	require.Equal(t, float64(30), dig(stored, "compute", "scaling", "scaleToZeroTimeout"))
	state = u.Properties
	requireNoChanges(t, l.diff(c.ID, state, updated, updated))

	// An update that does not touch the image does not resend it, so an image
	// managed outside Pulumi survives unrelated edits.
	scaled := with(updated, map[string]property.Value{"maxReplica": num(5)})
	l.f.resetCalls()
	l.update(c.ID, state, scaled, updated)
	body := decodeBody(t, l.f.callsMatching("PUT", "/v2/endpoint/me/ep")[0].Body)
	require.NotContains(t, body["model"], "image")
	require.Equal(t, float64(5), dig(l.f.endpoint("me/ep"), "compute", "scaling", "maxReplica"))
	state = l.read(c.ID, state, scaled).Properties

	// Clearing env vars sends an empty map instead of leaving them behind.
	cleared := without(scaled, "customImageEnvVars")
	d = l.diff(c.ID, state, cleared, scaled)
	require.Equal(t, pgp.Update, d.DetailedDiff["customImageEnvVars"].Kind)
	l.update(c.ID, state, cleared, scaled)
	require.Empty(t, dig(l.f.endpoint("me/ep"), "model", "env"))
}

func TestEndpoint_ReplaceOnImmutableChanges(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	inputs := endpointInputs()
	c := l.create(inputs)

	other := with(inputs, map[string]property.Value{"namespace": str("another-org")})
	d := l.diff(c.ID, c.Properties, other, inputs)
	require.Equal(t, pgp.UpdateReplace, d.DetailedDiff["namespace"].Kind)
	// The Hugging Face name is the resource name, so a replace must delete first.
	require.True(t, d.DeleteBeforeReplace)
}

func TestEndpoint_RefreshDetectsAndRepairsDrift(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	inputs := with(endpointInputs(), map[string]property.Value{"endpointType": str("authenticated"), "minReplica": num(1)})
	c := l.create(inputs)

	l.f.mutateEndpoint("me/ep", func(raw map[string]any) {
		asMap(raw["model"])["repository"] = "someone/else"
		asMap(raw["compute"])["instanceSize"] = "x4"
		asMap(asMap(raw["compute"])["scaling"])["minReplica"] = 2
		raw["type"] = "public"
		asMap(raw["provider"])["region"] = "eu-west-1"
	})

	r := l.read(c.ID, c.Properties, inputs)
	require.Equal(t, "someone/else", getStr(t, r.Inputs, "modelRepository"))
	require.Equal(t, "x4", getStr(t, r.Inputs, "instanceSize"))
	require.Equal(t, "eu-west-1", getStr(t, r.Inputs, "region"))
	require.Equal(t, "public", getStr(t, r.Inputs, "endpointType"))
	require.Equal(t, float64(2), r.Inputs.Get("minReplica").AsNumber())
	require.Equal(t, "someone/else", getStr(t, r.Properties, "modelRepository"))

	// The program's inputs now differ from the refreshed state...
	d := l.diff(c.ID, r.Properties, inputs, r.Inputs)
	require.True(t, d.HasChanges)
	for _, prop := range []string{"modelRepository", "instanceSize", "region", "endpointType", "minReplica"} {
		require.Equal(t, pgp.Update, d.DetailedDiff[prop].Kind, prop)
	}
	// ...and the update puts the endpoint back.
	l.update(c.ID, r.Properties, inputs, r.Inputs)
	stored := l.f.endpoint("me/ep")
	require.Equal(t, "org/model", dig(stored, "model", "repository"))
	require.Equal(t, "x2", dig(stored, "compute", "instanceSize"))
	require.Equal(t, "authenticated", stored["type"])
	require.Equal(t, "us-east-1", dig(stored, "provider", "region"))
	require.Equal(t, float64(1), dig(stored, "compute", "scaling", "minReplica"))
}

func TestEndpoint_DriftInUnsetDefaultsIsSurfaced(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	inputs := endpointInputs() // endpointType and replicas left at their defaults
	c := l.create(inputs)

	l.f.mutateEndpoint("me/ep", func(raw map[string]any) { raw["type"] = "public" })
	r := l.read(c.ID, c.Properties, inputs)
	require.Equal(t, "public", getStr(t, r.Properties, "endpointType"))
	d := l.diff(c.ID, r.Properties, inputs, r.Inputs)
	require.Equal(t, pgp.Update, d.DetailedDiff["endpointType"].Kind, "default authenticated must be restored")
}

func TestEndpoint_ImportByIDAlone(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "imported")
	l.f.addEndpoint("me/imported", map[string]any{
		"name": "imported",
		"type": "authenticated",
		"compute": map[string]any{
			"accelerator": "gpu", "instanceType": "nvidia-a10g", "instanceSize": "x1",
			"scaling": map[string]any{"minReplica": 0, "maxReplica": 2, "scaleToZeroTimeout": 15},
		},
		"model": map[string]any{
			"repository": "org/imported-model", "framework": "pytorch", "task": "text-generation", "revision": "deadbeef",
			"image": map[string]any{"custom": map[string]any{"url": "ghcr.io/x/y:1", "port": 8000, "healthRoute": "/up"}},
			"env":   map[string]any{"K": "V"},
		},
		"provider": map[string]any{"vendor": "gcp", "region": "us-central1"},
		"status":   map[string]any{"state": "running", "url": "https://imported.example"},
	})

	r := l.read("me/imported", property.NewMap(nil), property.NewMap(nil))
	require.Equal(t, "me/imported", r.ID)
	for k, want := range map[string]string{
		"modelRepository": "org/imported-model", "modelFramework": "pytorch", "modelTask": "text-generation",
		"modelRevision": "deadbeef", "vendor": "gcp", "region": "us-central1", "accelerator": "gpu",
		"instanceType": "nvidia-a10g", "instanceSize": "x1", "endpointType": "authenticated", "namespace": "me",
		"customImageUrl": "ghcr.io/x/y:1", "customImageHealthRoute": "/up",
	} {
		require.Equal(t, want, getStr(t, r.Inputs, k), k)
	}
	require.Equal(t, float64(8000), r.Inputs.Get("customImagePort").AsNumber())
	require.Equal(t, float64(0), r.Inputs.Get("minReplica").AsNumber())
	require.Equal(t, float64(2), r.Inputs.Get("maxReplica").AsNumber())
	require.Equal(t, float64(15), r.Inputs.Get("scaleToZeroTimeout").AsNumber())
	require.Equal(t, "V", r.Inputs.Get("customImageEnvVars").AsMap().Get("K").AsString())
	require.Equal(t, "https://imported.example", getStr(t, r.Properties, "url"))
	require.Equal(t, "running", getStr(t, r.Properties, "status"))

	// The imported inputs are a valid, complete program: no diff on the next up.
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: r.Inputs})
	require.NoError(t, err)
	require.Empty(t, chk.Failures)
	requireNoChanges(t, l.diff(r.ID, r.Properties, chk.Inputs, r.Inputs))
}

func TestEndpoint_DeletedOutsidePulumi(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	inputs := endpointInputs()
	c := l.create(inputs)

	l.f.deleteEndpointOutOfBand("me/ep")
	r := l.read(c.ID, c.Properties, inputs)
	require.Empty(t, r.ID, "an empty ID drops the resource from state")

	// destroy is quiet when the endpoint is already gone.
	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
}

func TestEndpoint_Delete(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	c := l.create(endpointInputs())
	require.NoError(t, l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties}))
	require.Nil(t, l.f.endpoint("me/ep"))
}

func TestEndpoint_CheckRejectsRemovedAndInvalidTypes(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	for _, bad := range []string{"protected", "internal"} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: with(endpointInputs(), map[string]property.Value{"endpointType": str(bad)})})
		require.NoError(t, err)
		require.Len(t, chk.Failures, 1, bad)
		require.Equal(t, "endpointType", chk.Failures[0].Property)
	}
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: with(endpointInputs(), map[string]property.Value{"endpointType": str("protected")})})
	require.NoError(t, err)
	require.Contains(t, chk.Failures[0].Reason, "authenticated")
	for _, ok := range []string{"public", "authenticated", "private"} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: with(endpointInputs(), map[string]property.Value{"endpointType": str(ok)})})
		require.NoError(t, err)
		require.Empty(t, chk.Failures, ok)
	}
}

func TestEndpoint_FakeRejectsProtectedType(t *testing.T) {
	// Guards the fake itself: the type enum must be enforced like the real API.
	l := newLC(t, "InferenceEndpoint", "ep")
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: with(endpointInputs(), map[string]property.Value{"endpointType": str("protected")})})
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestEndpoint_CreateFailureIsReported(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	l.f.failEndpoints = true
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: endpointInputs()})
	require.Error(t, err)
	// The endpoint exists on the Hub, so the failed create is recorded for repair.
	require.NotNil(t, l.f.endpoint("me/ep"))
}

func TestEndpoint_PreviewDoesNotCallTheAPI(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: endpointInputs(), DryRun: true})
	require.NoError(t, err)
	require.NotEmpty(t, c.ID)
	require.Empty(t, l.f.callsMatching("POST", "/v2/endpoint"))
	require.Empty(t, l.f.callsMatching("GET", "/api/whoami"))
}

func TestEndpoint_ReadWithoutTokenIsAnError(t *testing.T) {
	f := startFakeHF(t)
	s := serverFor(t, "")
	urn := hfUrn("InferenceEndpoint", "ep")
	f.resetCalls()
	_, err := s.Read(pgp.ReadRequest{ID: "me/ep", Urn: urn, Properties: pm(map[string]property.Value{"url": str("x")}), Inputs: endpointInputs()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "token")
	require.Empty(t, f.callsMatching("GET", ""))
}

func TestEndpoint_ResourceTokenOverridesProviderToken(t *testing.T) {
	f := startFakeHF(t)
	s := serverFor(t, "wrong-provider-token")
	urn := hfUrn("InferenceEndpoint", "ep")
	in := with(endpointInputs(), map[string]property.Value{"token": str(f.token)})
	c, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: in})
	require.NoError(t, err)
	require.Equal(t, "me/ep", c.ID)
}

func TestEndpoint_NoVersionProperty(t *testing.T) {
	// infer drops an input named "version" from every diff.
	s := serverFor(t, "x")
	resp, err := s.GetSchema(pgp.GetSchemaRequest{})
	require.NoError(t, err)
	var spec struct {
		Resources map[string]struct {
			InputProperties map[string]any `json:"inputProperties"`
			Properties      map[string]any `json:"properties"`
		} `json:"resources"`
	}
	require.NoError(t, jsonUnmarshal(resp.Schema, &spec))
	require.NotEmpty(t, spec.Resources)
	for name, r := range spec.Resources {
		require.NotContains(t, r.InputProperties, "version", name)
		require.NotContains(t, r.Properties, "version", name)
	}
}

func TestEndpoint_TagsLifecycle(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	in := with(endpointInputs(), map[string]property.Value{"tags": arr(str("prod"), str("team-a"))})
	c := l.create(in)
	body := decodeBody(t, l.f.callsMatching("POST", "/v2/endpoint/me")[0].Body)
	require.Equal(t, []any{"prod", "team-a"}, body["tags"])
	require.Equal(t, []any{"prod", "team-a"}, l.f.endpoint("me/ep")["tags"])
	requireNoChanges(t, l.diff(c.ID, c.Properties, in, in))

	// The API may return the tags in another order: that is not drift.
	l.f.mutateEndpoint("me/ep", func(raw map[string]any) { raw["tags"] = []any{"team-a", "prod"} })
	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))
	reordered := with(in, map[string]property.Value{"tags": arr(str("team-a"), str("prod"))})
	requireNoChanges(t, l.diff(c.ID, rd.Properties, reordered, in))

	// Changing the tags is an update that sends them.
	next := with(in, map[string]property.Value{"tags": arr(str("prod"), str("team-b"))})
	d := l.diff(c.ID, rd.Properties, next, in)
	require.Equal(t, pgp.Update, d.DetailedDiff["tags"].Kind)
	require.False(t, d.DeleteBeforeReplace)
	l.f.resetCalls()
	u := l.update(c.ID, rd.Properties, next, in)
	put := decodeBody(t, l.f.callsMatching("PUT", "/v2/endpoint/me/ep")[0].Body)
	require.Equal(t, []any{"prod", "team-b"}, put["tags"])
	requireNoChanges(t, l.diff(c.ID, u.Properties, next, next))

	// An empty list clears them.
	none := with(in, map[string]property.Value{"tags": arr()})
	require.Equal(t, pgp.Update, l.diff(c.ID, u.Properties, none, next).DetailedDiff["tags"].Kind)
	l.f.resetCalls()
	l.update(c.ID, u.Properties, none, next)
	put = decodeBody(t, l.f.callsMatching("PUT", "/v2/endpoint/me/ep")[0].Body)
	require.Equal(t, []any{}, put["tags"])
	require.Empty(t, l.f.endpoint("me/ep")["tags"])
}

func TestEndpoint_TagsDriftUnsetAndImport(t *testing.T) {
	l := newLC(t, "InferenceEndpoint", "ep")
	in := endpointInputs()
	c := l.create(in)
	require.NotContains(t, decodeBody(t, l.f.callsMatching("POST", "/v2/endpoint/me")[0].Body), "tags")

	// Unset means unmanaged: tags added elsewhere are not a diff.
	l.f.mutateEndpoint("me/ep", func(raw map[string]any) { raw["tags"] = []any{"added-elsewhere"} })
	rd := l.read(c.ID, c.Properties, in)
	requireNoChanges(t, l.diff(c.ID, rd.Properties, in, rd.Inputs))
	l.f.resetCalls()
	l.update(c.ID, rd.Properties, with(in, map[string]property.Value{"modelTask": str("fill-mask")}), in)
	require.NotContains(t, decodeBody(t, l.f.callsMatching("PUT", "/v2/endpoint/me/ep")[0].Body), "tags")

	// Managed tags removed elsewhere are drift to repair.
	managed := with(in, map[string]property.Value{"tags": arr(str("a"))})
	m := newLC(t, "InferenceEndpoint", "ep2")
	mc := m.create(managed)
	m.f.mutateEndpoint("me/ep2", func(raw map[string]any) { delete(raw, "tags") })
	mrd := m.read(mc.ID, mc.Properties, managed)
	require.Empty(t, strs(mrd.Inputs, "tags"))
	require.Equal(t, pgp.Update, m.diff(mc.ID, mrd.Properties, managed, mrd.Inputs).DetailedDiff["tags"].Kind)

	// Import adopts the API's tags.
	m.f.mutateEndpoint("me/ep2", func(raw map[string]any) { raw["tags"] = []any{"x", "y"} })
	imp := m.read(mc.ID, empty(), empty())
	require.Equal(t, []string{"x", "y"}, strs(imp.Inputs, "tags"))
	chk := m.checked(imp.Inputs)
	requireNoChanges(t, m.diff(imp.ID, imp.Properties, chk, imp.Inputs))
}
