package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// errTokenRequired is returned by every operation that needs the API and has no token.
var errTokenRequired = errors.New("huggingface: API token is required (set the token input, the provider token config, or the HUGGINGFACE_TOKEN env var)")

func resolveToken(resourceToken *string, cfgToken *string) string {
	if resourceToken != nil && *resourceToken != "" {
		return *resourceToken
	}
	if cfgToken != nil && *cfgToken != "" {
		return *cfgToken
	}
	return ""
}

// clientFor builds an API client from the resource token or the provider config.
func clientFor(ctx context.Context, resourceToken *string) (*hfClient, string, error) {
	cfg := infer.GetConfig[ProviderConfig](ctx)
	token := resolveToken(resourceToken, cfg.Token)
	if token == "" {
		return nil, "", errTokenRequired
	}
	return newHFClient(token), token, nil
}

func resolveNamespace(ns *string, token string, client *hfClient) (string, error) {
	if ns != nil && *ns != "" {
		return *ns, nil
	}
	// Whoami call to get the authenticated user's namespace.
	body, _, err := client.do("GET", hubBase+"/api/whoami-v2", nil)
	if err != nil {
		return "", fmt.Errorf("fetch authenticated user namespace: %w", err)
	}
	var who struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &who); err != nil || who.Name == "" {
		return "", fmt.Errorf("could not determine namespace from token; set namespace explicitly")
	}
	return who.Name, nil
}

// SplitID parses a "namespace/name" resource ID into its components.
// Exported for use in tests.
func SplitID(id string) (namespace, name string, err error) {
	return splitID(id)
}

func splitID(id string) (namespace, name string, err error) {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' {
			if i == 0 || i == len(id)-1 {
				break
			}
			return id[:i], id[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid resource ID %q: expected namespace/name", id)
}

func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ---------------------------------------------------------------------------
// Optional-property helpers
// ---------------------------------------------------------------------------

// ptr returns a pointer to a copy of v.
func ptr[T any](v T) *T { return &v }

// orDefault resolves an optional input to its effective value.
func orDefault[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// refreshOpt computes the value of an optional input after a refresh.
//
//   - got/hasGot: what the API reports. When it reports nothing the prior
//     input is kept.
//   - imported: there is no prior input (import), so everything the API
//     reports becomes an input.
//   - def: the effective default when the input is unset. If the prior input
//     is unset and the API still reports the default, the input stays unset;
//     if the API reports something else (drift) it is surfaced so the next
//     update puts the default back. A nil def marks a property that is
//     unmanaged while unset: it stays unset.
func refreshOpt[T comparable](prior *T, got T, hasGot, imported bool, def *T) *T {
	if !hasGot {
		return prior
	}
	if imported {
		return ptr(got)
	}
	if prior == nil {
		if def != nil && got != *def {
			return ptr(got)
		}
		return nil
	}
	return ptr(got)
}

// ---------------------------------------------------------------------------
// Diff helper
// ---------------------------------------------------------------------------

// diffSet accumulates a detailed diff. Replacements always set
// DeleteBeforeReplace because the Hugging Face name is the Pulumi resource
// name, so a replacement that keeps the name cannot be created first.
type diffSet struct {
	detail     map[string]pgp.PropertyDiff
	hasReplace bool
}

func newDiffSet() *diffSet {
	return &diffSet{detail: map[string]pgp.PropertyDiff{}}
}

func (d *diffSet) update(prop string, changed bool) {
	if changed {
		d.detail[prop] = pgp.PropertyDiff{Kind: pgp.Update}
	}
}

func (d *diffSet) replace(prop string, changed bool) {
	if changed {
		d.detail[prop] = pgp.PropertyDiff{Kind: pgp.UpdateReplace}
		d.hasReplace = true
	}
}

func (d *diffSet) response() infer.DiffResponse {
	return infer.DiffResponse{
		HasChanges:          len(d.detail) > 0,
		DetailedDiff:        d.detail,
		DeleteBeforeReplace: d.hasReplace,
	}
}

// ptrChanged reports a change between two optional values, where an unset
// "new" value means the property is unmanaged (never a change).
func ptrChanged[T comparable](old, new *T) bool {
	if new == nil {
		return false
	}
	return old == nil || *old != *new
}

// namespaceChanged reports whether an explicit namespace differs from the one
// the resource lives in (taken from its ID). An unset namespace never changes.
func namespaceChanged(idNamespace string, ns *string) bool {
	return ns != nil && *ns != "" && idNamespace != *ns
}
