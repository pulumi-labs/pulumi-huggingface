package provider

import (
	"encoding/json"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// invoke calls a lookup function through the provider RPC surface.
func invoke(t *testing.T, s integration.Server, token string, args map[string]property.Value) (property.Map, error) {
	t.Helper()
	resp, err := s.Invoke(pgp.InvokeRequest{Token: tokens.Type("huggingface:index:" + token), Args: pm(args)})
	if err != nil {
		return property.Map{}, err
	}
	return resp.Return, nil
}

// checked runs Check and returns the validated inputs.
func (l *lc) checked(inputs property.Map) property.Map {
	l.t.Helper()
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: inputs})
	require.NoError(l.t, err)
	require.Empty(l.t, chk.Failures)
	return chk.Inputs
}

func (l *lc) checkFailures(inputs property.Map) []pgp.CheckFailure {
	l.t.Helper()
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: inputs})
	require.NoError(l.t, err)
	return chk.Failures
}

func (l *lc) del(id string, state property.Map) {
	l.t.Helper()
	require.NoError(l.t, l.s.Delete(pgp.DeleteRequest{ID: id, Urn: l.urn, Properties: state}))
}

func empty() property.Map { return property.NewMap(nil) }

func strs(m property.Map, key string) []string {
	var out []string
	for _, v := range m.Get(key).AsArray().AsSlice() {
		out = append(out, v.AsString())
	}
	return out
}

// TestSchema_NoInputNamedVersionAndEveryTokenGenerates asserts the schema
// has no input named "version" (pulumi-go-provider#704) and that every
// resource and function generates.
func TestSchema_NoInputNamedVersionAndEveryTokenGenerates(t *testing.T) {
	s := serverFor(t, fakeToken)
	resp, err := s.GetSchema(pgp.GetSchemaRequest{})
	require.NoError(t, err)
	var spec struct {
		Resources map[string]struct {
			InputProperties map[string]json.RawMessage `json:"inputProperties"`
		} `json:"resources"`
		Functions map[string]json.RawMessage `json:"functions"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Schema), &spec))
	for _, r := range []string{"Webhook", "Collection", "CollectionItem", "RepositoryBranch", "RepositoryTag", "Bucket",
		"InferenceEndpoint", "Space", "Repository"} {
		res, ok := spec.Resources["huggingface:index:"+r]
		require.True(t, ok, r)
		require.NotContains(t, res.InputProperties, "version", r)
	}
	for _, fn := range []string{"getWhoami", "getInferenceEndpoint", "listInferenceEndpoints", "getSpace", "getRepository",
		"getWebhook", "getCollection", "getCollectionItem", "getRepositoryBranch", "getRepositoryTag", "getBucket"} {
		require.Contains(t, spec.Functions, "huggingface:index:"+fn)
	}
}
