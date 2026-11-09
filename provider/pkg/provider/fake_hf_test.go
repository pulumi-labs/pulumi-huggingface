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
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHF is an in-memory Hugging Face server covering the Inference Endpoints
// API and the Hub API calls the provider makes. It follows the real API
// semantics (as documented by the OpenAPI specs and huggingface_hub):
//
//   - endpoint type is one of public|authenticated|private ("protected" is rejected),
//   - model.image is required and holds exactly one variant,
//   - the health route is custom.healthRoute and env is model.env,
//   - private link configuration is privateService{accountId,region,shared},
//   - updates accept compute, model, provider and type,
//   - Space secrets/variables are written one {key,value,description} object
//     per POST and removed with DELETE {key},
//   - repo visibility is changed with PUT .../settings,
//   - the Space runtime is runtime.hardware.{current,requested} and runtime.gcTimeout.
type fakeHF struct {
	t      *testing.T
	server *httptest.Server
	token  string

	mu        sync.Mutex
	endpoints map[string]*fakeEndpoint // "ns/name"
	repos     map[string]*fakeRepo     // "type/ns/name"
	calls     []fakeCall

	webhooks    map[string]*fakeWebhook    // by ID
	collections map[string]*fakeCollection // by slug
	buckets     map[string]*fakeBucket     // "ns/name"
	nextID      int

	// pollsUntilRunning is how many GETs an endpoint answers with a
	// transitional state before reaching running.
	pollsUntilRunning int
	// failEndpoints makes new endpoints end in the failed state.
	failEndpoints bool
	// failOn makes matching requests answer 500: entries are "METHOD path-substring".
	failOn []string
}

type fakeCall struct {
	Method string
	Path   string
	Body   string
}

type fakeEndpoint struct {
	raw       map[string]any // the stored representation (as returned by GET)
	polls     int
	stateNext string // state reached after pollsUntilRunning GETs
}

type fakeRepo struct {
	typ, ns, name string
	private       bool
	sdk           string
	hardware      string
	gcTimeout     *int
	secrets       map[string]map[string]any
	variables     map[string]map[string]any
	gated         string            // "", "auto" or "manual"
	branches      map[string]string // name -> commit
	tags          map[string]string // name -> commit
	tagObjects    map[string]string // annotated tag name -> tag object id (what /refs reports)
}

var fakeHardwareFlavors = []string{
	"cpu-basic", "cpu-upgrade", "zero-a10g", "t4-small", "t4-medium", "l4x1", "l4x4", "l40sx1", "l40sx4", "l40sx8",
	"a10g-small", "a10g-large", "a10g-largex2", "a10g-largex4", "a100-large", "a100x4", "a100x8",
	"rtx-pro-6000", "rtx-pro-6000x2", "rtx-pro-6000x4", "rtx-pro-6000x8",
}

const fakeToken = "hf_test_token"

func startFakeHF(t *testing.T) *fakeHF {
	t.Helper()
	f := &fakeHF{
		t:                 t,
		token:             fakeToken,
		endpoints:         map[string]*fakeEndpoint{},
		repos:             map[string]*fakeRepo{},
		webhooks:          map[string]*fakeWebhook{},
		collections:       map[string]*fakeCollection{},
		buckets:           map[string]*fakeBucket{},
		pollsUntilRunning: 2,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)

	oldEP, oldHub, oldPoll, oldWait := endpointsBase, hubBase, endpointPollInterval, endpointWaitTimeout
	endpointsBase, hubBase = f.server.URL+"/v2", f.server.URL
	endpointPollInterval, endpointWaitTimeout = time.Millisecond, 5*time.Second
	t.Cleanup(func() {
		endpointsBase, hubBase, endpointPollInterval, endpointWaitTimeout = oldEP, oldHub, oldPoll, oldWait
	})
	return f
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (f *fakeHF) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{r.Method, r.URL.Path, string(body)})

	if r.Header.Get("Authorization") != "Bearer "+f.token {
		apiError(w, http.StatusUnauthorized, "Invalid username or password.")
		return
	}
	path := r.URL.Path
	for _, rule := range f.failOn {
		if m, sub, ok := strings.Cut(rule, " "); ok && m == r.Method && strings.Contains(path, sub) {
			apiError(w, http.StatusInternalServerError, "injected failure")
			return
		}
	}
	switch {
	case path == "/api/whoami-v2":
		writeJSON(w, 200, map[string]any{"name": "me", "type": "user", "fullname": "Me Myself", "orgs": []map[string]any{{"name": "acme"}}})
	case strings.HasPrefix(path, "/api/settings/webhooks"):
		f.handleWebhook(w, r, strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/settings/webhooks"), "/"), "/"), body)
	case strings.HasPrefix(path, "/api/collections"):
		f.handleCollection(w, r, strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/collections"), "/"), "/"), body)
	case strings.HasPrefix(path, "/api/buckets/"):
		f.handleBucket(w, r, strings.Split(strings.TrimPrefix(path, "/api/buckets/"), "/"), body)
	case strings.HasPrefix(path, "/v2/endpoint/"):
		f.handleEndpoint(w, r, strings.Split(strings.TrimPrefix(path, "/v2/endpoint/"), "/"), body)
	case path == "/api/repos/create" && r.Method == "POST":
		f.handleCreateRepo(w, body)
	case path == "/api/repos/delete" && r.Method == "DELETE":
		f.handleDeleteRepo(w, body)
	case strings.HasPrefix(path, "/api/"):
		f.handleRepo(w, r, strings.Split(strings.TrimPrefix(path, "/api/"), "/"), body)
	default:
		apiError(w, 404, "not found")
	}
}

// ---------------------------------------------------------------------------
// Inference Endpoints
// ---------------------------------------------------------------------------

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func validateEndpointCommon(body map[string]any, creating bool) string {
	if tp, ok := body["type"]; ok {
		if s, _ := tp.(string); !slices.Contains([]string{"public", "authenticated", "private"}, s) {
			return fmt.Sprintf("invalid type %q: expected one of public, authenticated, private", tp)
		}
	}
	model := asMap(body["model"])
	if creating {
		if model == nil {
			return "model is required"
		}
		for _, k := range []string{"repository", "framework"} {
			if s, _ := model[k].(string); s == "" {
				return "model." + k + " is required"
			}
		}
		if asMap(body["compute"]) == nil || asMap(body["provider"]) == nil {
			return "compute and provider are required"
		}
	}
	if model != nil {
		img, present := model["image"]
		if creating && !present {
			return "model.image is required"
		}
		if present {
			im := asMap(img)
			if len(im) != 1 {
				return "model.image must hold exactly one variant"
			}
			if c := asMap(im["custom"]); c != nil {
				if u, _ := c["url"].(string); u == "" {
					return "model.image.custom.url is required"
				}
			}
		}
	}
	return ""
}

// merge deep-merges src into dst (maps merge, everything else replaces),
// which is how a partial PUT behaves.
func merge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok && k != "image" && k != "env" {
				merge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

func (f *fakeHF) endpointView(ep *fakeEndpoint) map[string]any {
	// GET advances the lifecycle.
	ep.polls++
	st := asMap(ep.raw["status"])
	if st["state"] == "pending" || st["state"] == "initializing" || st["state"] == "updating" {
		if ep.polls > f.pollsUntilRunning {
			st["state"] = ep.stateNext
			if ep.stateNext == "running" {
				st["url"] = fmt.Sprintf("https://%s.endpoints.huggingface.cloud", ep.raw["name"])
			}
			st["message"] = ""
		} else if st["state"] == "pending" {
			st["state"] = "initializing"
		}
	}
	return ep.raw
}

func (f *fakeHF) handleEndpoint(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			apiError(w, 400, "bad json")
			return
		}
	}
	switch {
	case len(parts) == 1 && r.Method == "POST":
		ns := parts[0]
		if msg := validateEndpointCommon(body, true); msg != "" {
			apiError(w, 400, msg)
			return
		}
		name, _ := body["name"].(string)
		key := ns + "/" + name
		if _, exists := f.endpoints[key]; exists {
			apiError(w, 409, "endpoint already exists")
			return
		}
		// Only declared fields are kept: unknown fields such as a top-level
		// accountId, custom.env or custom.health are silently dropped, as the
		// real API does.
		model := asMap(body["model"])
		stored := map[string]any{
			"name":     name,
			"type":     body["type"],
			"compute":  body["compute"],
			"provider": body["provider"],
			"model": map[string]any{
				"repository": model["repository"], "framework": model["framework"], "task": model["task"],
				"revision": model["revision"], "image": model["image"], "env": model["env"],
			},
			"status": map[string]any{"state": "pending", "message": "Endpoint is pending", "url": ""},
		}
		if tags, ok := body["tags"]; ok {
			stored["tags"] = tags
		}
		if ps := asMap(body["privateService"]); ps != nil {
			stored["privateService"] = map[string]any{"accountId": ps["accountId"], "region": ps["region"], "shared": false}
		}
		next := "running"
		if f.failEndpoints {
			next = "failed"
		}
		f.endpoints[key] = &fakeEndpoint{raw: stored, stateNext: next}
		writeJSON(w, 200, stored)
	case len(parts) == 1 && r.Method == "GET":
		items := []any{}
		for _, k := range slices.Sorted(maps.Keys(f.endpoints)) {
			if strings.HasPrefix(k, parts[0]+"/") {
				items = append(items, f.endpoints[k].raw)
			}
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case len(parts) == 2:
		key := parts[0] + "/" + parts[1]
		ep, ok := f.endpoints[key]
		if !ok {
			apiError(w, 404, "endpoint not found")
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, f.endpointView(ep))
		case "DELETE":
			delete(f.endpoints, key)
			w.WriteHeader(202)
		case "PUT":
			if msg := validateEndpointCommon(body, false); msg != "" {
				apiError(w, 400, msg)
				return
			}
			merge(ep.raw, body)
			ep.polls = 0
			asMap(ep.raw["status"])["state"] = "updating"
			writeJSON(w, 200, ep.raw)
		default:
			apiError(w, 405, "method not allowed")
		}
	default:
		apiError(w, 404, "not found")
	}
}

// ---------------------------------------------------------------------------
// Hub repositories and Spaces
// ---------------------------------------------------------------------------

func repoKey(typ, ns, name string) string { return typ + "/" + ns + "/" + name }

func (f *fakeHF) handleCreateRepo(w http.ResponseWriter, raw []byte) {
	var body struct {
		Name         string           `json:"name"`
		Type         string           `json:"type"`
		Organization string           `json:"organization"`
		Private      bool             `json:"private"`
		SDK          string           `json:"sdk"`
		Hardware     string           `json:"hardware"`
		SleepTime    *int             `json:"sleepTimeSeconds"`
		Secrets      []map[string]any `json:"secrets"`
		Variables    []map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		apiError(w, 400, "bad json")
		return
	}
	if !slices.Contains([]string{"model", "dataset", "space"}, body.Type) {
		apiError(w, 400, "invalid repo type")
		return
	}
	ns := body.Organization
	if ns == "" {
		ns = "me"
	}
	if body.Type == "space" {
		if !slices.Contains([]string{"gradio", "streamlit", "docker", "static"}, body.SDK) {
			apiError(w, 400, "invalid space sdk")
			return
		}
		if body.Hardware != "" && !slices.Contains(fakeHardwareFlavors, body.Hardware) {
			apiError(w, 400, "invalid hardware flavor "+body.Hardware)
			return
		}
	}
	key := repoKey(body.Type, ns, body.Name)
	if _, ok := f.repos[key]; ok {
		apiError(w, 409, "You already created this repo")
		return
	}
	repo := &fakeRepo{
		typ: body.Type, ns: ns, name: body.Name, private: body.Private,
		secrets: map[string]map[string]any{}, variables: map[string]map[string]any{},
		branches: map[string]string{"main": f.commit()}, tags: map[string]string{}, tagObjects: map[string]string{},
	}
	if body.Type == "space" {
		repo.sdk = body.SDK
		repo.hardware = body.Hardware
		if repo.hardware == "" {
			repo.hardware = "cpu-basic"
		}
		repo.gcTimeout = body.SleepTime
		for _, s := range body.Secrets {
			repo.secrets[fmt.Sprint(s["key"])] = map[string]any{"description": s["description"], "value": s["value"]}
		}
		for _, v := range body.Variables {
			repo.variables[fmt.Sprint(v["key"])] = map[string]any{"value": v["value"], "description": v["description"]}
		}
	}
	f.repos[key] = repo
	prefix := ""
	switch body.Type {
	case "dataset":
		prefix = "datasets/"
	case "space":
		prefix = "spaces/"
	}
	writeJSON(w, 200, map[string]any{
		"url":  fmt.Sprintf("https://huggingface.co/%s%s/%s", prefix, ns, body.Name),
		"name": ns + "/" + body.Name,
		// The live create response carries an internal ObjectId in id, not
		// namespace/name (#1549).
		"id": "65f0c0ffee0000000000abcd",
	})
}

func (f *fakeHF) handleDeleteRepo(w http.ResponseWriter, raw []byte) {
	var body struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Organization string `json:"organization"`
	}
	_ = json.Unmarshal(raw, &body)
	key := repoKey(body.Type, body.Organization, body.Name)
	if _, ok := f.repos[key]; !ok {
		apiError(w, 404, "Repository not found")
		return
	}
	delete(f.repos, key)
	w.WriteHeader(200)
}

var apiTypeFor = map[string]string{"models": "model", "datasets": "dataset", "spaces": "space"}

func (f *fakeHF) handleRepo(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	if len(parts) < 3 {
		apiError(w, 404, "not found")
		return
	}
	typ, ok := apiTypeFor[parts[0]]
	if !ok {
		apiError(w, 404, "not found")
		return
	}
	repo, ok := f.repos[repoKey(typ, parts[1], parts[2])]
	if !ok {
		apiError(w, 404, "Repository not found")
		return
	}
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			// A JSON array (the old bulk secrets/variables shape) lands here.
			apiError(w, 400, "expected a JSON object")
			return
		}
	}
	sub := ""
	if len(parts) > 3 {
		sub = parts[3]
	}
	switch {
	case sub == "" && r.Method == "GET":
		out := map[string]any{"id": repo.ns + "/" + repo.name, "private": repo.private, "gated": false}
		if repo.gated != "" {
			out["gated"] = repo.gated
		}
		if typ == "space" {
			out["sdk"] = repo.sdk
			rt := map[string]any{
				"stage":    "RUNNING",
				"hardware": map[string]any{"current": repo.hardware, "requested": nil},
			}
			if repo.gcTimeout != nil {
				rt["gcTimeout"] = *repo.gcTimeout
			}
			out["runtime"] = rt
		}
		writeJSON(w, 200, out)
	case sub == "settings" && r.Method == "PUT":
		p, hasPrivate := body["private"].(bool)
		g, hasGated := body["gated"]
		if !hasPrivate && !hasGated {
			apiError(w, 400, "At least one setting must be updated.")
			return
		}
		if hasGated {
			switch gv := g.(type) {
			case bool:
				if gv {
					apiError(w, 400, "gated must be 'auto', 'manual' or false")
					return
				}
				repo.gated = ""
			case string:
				if gv != "auto" && gv != "manual" {
					apiError(w, 400, "gated must be 'auto', 'manual' or false")
					return
				}
				repo.gated = gv
			default:
				apiError(w, 400, "gated must be 'auto', 'manual' or false")
				return
			}
		}
		if hasPrivate {
			repo.private = p
		}
		w.WriteHeader(200)
	case sub == "refs" || sub == "branch" || sub == "tag" || sub == "revision":
		f.handleRefs(w, r, repo, sub, parts[4:], body)
	case typ != "space":
		apiError(w, 404, "not found")
	case sub == "hardware" && r.Method == "POST":
		flavor, _ := body["flavor"].(string)
		if !slices.Contains(fakeHardwareFlavors, flavor) {
			apiError(w, 400, "invalid hardware flavor "+flavor)
			return
		}
		repo.hardware = flavor
		writeJSON(w, 200, map[string]any{"stage": "RUNNING", "hardware": map[string]any{"current": flavor, "requested": flavor}})
	case sub == "sleeptime" && r.Method == "POST":
		secs, ok := body["seconds"].(float64)
		if !ok {
			apiError(w, 400, "seconds is required")
			return
		}
		n := int(secs)
		repo.gcTimeout = &n
		writeJSON(w, 200, map[string]any{"stage": "RUNNING", "gcTimeout": n})
	case sub == "secrets" || sub == "variables":
		f.handleKeyed(w, r, repo, sub, body)
	default:
		apiError(w, 404, "not found")
	}
}

func (f *fakeHF) handleKeyed(w http.ResponseWriter, r *http.Request, repo *fakeRepo, sub string, body map[string]any) {
	store := repo.secrets
	if sub == "variables" {
		store = repo.variables
	}
	view := func() map[string]any {
		out := map[string]any{}
		for k, v := range store {
			e := map[string]any{"description": v["description"], "updatedAt": "2026-01-01T00:00:00Z"}
			if sub == "variables" { // secret values are never returned
				e["value"] = v["value"]
			}
			out[k] = e
		}
		return out
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, view())
	case "POST":
		key, _ := body["key"].(string)
		if key == "" || body["value"] == nil {
			apiError(w, 400, "key and value are required")
			return
		}
		store[key] = map[string]any{"value": body["value"], "description": body["description"]}
		writeJSON(w, 200, view())
	case "DELETE":
		key, _ := body["key"].(string)
		if _, ok := store[key]; !ok {
			apiError(w, 404, "key not found")
			return
		}
		delete(store, key)
		writeJSON(w, 200, view())
	default:
		apiError(w, 405, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Test helpers over the fake's state
// ---------------------------------------------------------------------------

func (f *fakeHF) resetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// callsMatching returns the recorded calls with the given method whose path
// contains the substring.
func (f *fakeHF) callsMatching(method, pathPart string) []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, c := range f.calls {
		if c.Method == method && strings.Contains(c.Path, pathPart) {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeHF) endpoint(key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ep, ok := f.endpoints[key]; ok {
		b, _ := json.Marshal(ep.raw) // deep copy
		var out map[string]any
		_ = json.Unmarshal(b, &out)
		return out
	}
	return nil
}

func (f *fakeHF) mutateEndpoint(key string, fn func(raw map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.endpoints[key].raw)
}

func (f *fakeHF) mutateRepo(typ, ns, name string, fn func(r *fakeRepo)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.repos[repoKey(typ, ns, name)])
}

func (f *fakeHF) repo(typ, ns, name string) *fakeRepo {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.repos[repoKey(typ, ns, name)]
	if !ok {
		return nil
	}
	cp := *r
	cp.secrets, cp.variables = map[string]map[string]any{}, map[string]map[string]any{}
	for k, v := range r.secrets {
		cp.secrets[k] = v
	}
	for k, v := range r.variables {
		cp.variables[k] = v
	}
	return &cp
}

func (f *fakeHF) deleteEndpointOutOfBand(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.endpoints, key)
}

func (f *fakeHF) deleteRepoOutOfBand(typ, ns, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.repos, repoKey(typ, ns, name))
}

func (f *fakeHF) addRepo(typ, ns, name string, mutate func(r *fakeRepo)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := &fakeRepo{typ: typ, ns: ns, name: name, secrets: map[string]map[string]any{}, variables: map[string]map[string]any{},
		branches: map[string]string{"main": f.commit()}, tags: map[string]string{}, tagObjects: map[string]string{}}
	if typ == "space" {
		r.sdk, r.hardware = "gradio", "cpu-basic"
	}
	if mutate != nil {
		mutate(r)
	}
	f.repos[repoKey(typ, ns, name)] = r
}

func (f *fakeHF) addEndpoint(key string, raw map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.endpoints[key] = &fakeEndpoint{raw: raw, stateNext: "running", polls: 1 << 20}
}
