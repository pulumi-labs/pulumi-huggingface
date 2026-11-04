package provider

import (
	"encoding/json"
	"testing"

	"github.com/blang/semver"
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// These tests drive every resource through the real provider RPC surface
// (integration.Server) against the in-memory fake in fake_hf_test.go.

func pm(kv map[string]property.Value) property.Map { return property.NewMap(kv) }

func str(s string) property.Value { return property.New(s) }

func num(n int) property.Value { return property.New(float64(n)) }

func boolean(b bool) property.Value { return property.New(b) }

func strMap(m map[string]string) property.Value {
	out := map[string]property.Value{}
	for k, v := range m {
		out[k] = property.New(v)
	}
	return property.New(out)
}

func arr(vals ...property.Value) property.Value { return property.New(property.NewArray(vals)) }

func obj(kv map[string]property.Value) property.Value { return property.New(kv) }

func hfUrn(typ, name string) resource.URN {
	return resource.NewURN("stack", "proj", "", tokens.Type("huggingface:index:"+typ), name)
}

// serverFor returns a provider server configured with the fake's token.
func serverFor(t *testing.T, token string) integration.Server {
	t.Helper()
	t.Setenv("HUGGINGFACE_TOKEN", "")
	p, err := New()
	require.NoError(t, err)
	s, err := integration.NewServer(t.Context(), "huggingface", semver.MustParse("0.0.1"), integration.WithProvider(p))
	require.NoError(t, err)
	cfg := map[string]property.Value{}
	if token != "" {
		cfg["token"] = str(token)
	}
	require.NoError(t, s.Configure(pgp.ConfigureRequest{Args: pm(cfg)}))
	return s
}

func getStr(t *testing.T, m property.Map, key string) string {
	t.Helper()
	v, ok := m.GetOk(key)
	require.True(t, ok, "property %q missing from %v", key, m)
	return v.AsString()
}

func hasProp(m property.Map, key string) bool {
	_, ok := m.GetOk(key)
	return ok
}

func requireNoChanges(t *testing.T, d pgp.DiffResponse) {
	t.Helper()
	require.False(t, d.HasChanges, "unexpected diff: %+v", d.DetailedDiff)
}

func with(m property.Map, kv map[string]property.Value) property.Map {
	for k, v := range kv {
		m = m.Set(k, v)
	}
	return m
}

func without(m property.Map, keys ...string) property.Map {
	for _, k := range keys {
		m = m.Delete(k)
	}
	return m
}

// lc bundles the pieces every lifecycle step needs.
type lc struct {
	t   *testing.T
	s   integration.Server
	f   *fakeHF
	urn resource.URN
}

func newLC(t *testing.T, typ, name string) *lc {
	t.Helper()
	f := startFakeHF(t)
	return &lc{t: t, s: serverFor(t, f.token), f: f, urn: hfUrn(typ, name)}
}

func (l *lc) create(inputs property.Map) pgp.CreateResponse {
	l.t.Helper()
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: inputs})
	require.NoError(l.t, err)
	require.Empty(l.t, chk.Failures)
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: chk.Inputs})
	require.NoError(l.t, err)
	require.NotEmpty(l.t, c.ID)
	return c
}

func (l *lc) diff(id string, state, inputs, oldInputs property.Map) pgp.DiffResponse {
	l.t.Helper()
	d, err := l.s.Diff(pgp.DiffRequest{ID: id, Urn: l.urn, State: state, Inputs: inputs, OldInputs: oldInputs})
	require.NoError(l.t, err)
	return d
}

func (l *lc) update(id string, state, inputs, oldInputs property.Map) pgp.UpdateResponse {
	l.t.Helper()
	u, err := l.s.Update(pgp.UpdateRequest{ID: id, Urn: l.urn, State: state, Inputs: inputs, OldInputs: oldInputs})
	require.NoError(l.t, err)
	return u
}

func (l *lc) read(id string, state, inputs property.Map) pgp.ReadResponse {
	l.t.Helper()
	r, err := l.s.Read(pgp.ReadRequest{ID: id, Urn: l.urn, Properties: state, Inputs: inputs})
	require.NoError(l.t, err)
	return r
}

// dig walks nested JSON maps.
func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &out), body)
	return out
}
