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
	"net/http"
	"slices"
	"strings"
)

// This file extends the fake Hugging Face server with the Hub APIs behind
// Webhook, Collection, CollectionItem, RepositoryBranch, RepositoryTag and
// Bucket. The shapes follow huggingface_hub (hf_api.py): webhooks live under
// /api/settings/webhooks and answer {"webhook": {...}}, collections under
// /api/collections (PATCH answers {"data": ...}), git refs under
// /api/{type}s/{id}/{refs,branch/<b>,tag/<rev>}, buckets under
// /api/buckets/{ns}/{name}.

// commit returns a fresh fake commit SHA. The caller holds f.mu.
func (f *fakeHF) commit() string {
	f.nextID++
	return fmt.Sprintf("%040x", f.nextID)
}

// newID returns a fresh object ID. The caller holds f.mu.
func (f *fakeHF) newID(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s%024x", prefix, f.nextID)
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

type fakeWebhook struct {
	id       string
	url      string
	watched  []map[string]any
	domains  []string
	disabled bool
	secret   string
}

func (wh *fakeWebhook) view() map[string]any {
	return map[string]any{"webhook": map[string]any{
		"id": wh.id, "url": wh.url, "watched": wh.watched, "domains": wh.domains, "disabled": wh.disabled,
	}}
}

func validWatched(raw any) ([]map[string]any, string) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, "watched is required"
	}
	var out []map[string]any
	for _, it := range items {
		m := asMap(it)
		t, _ := m["type"].(string)
		n, _ := m["name"].(string)
		if !slices.Contains([]string{"bucket", "dataset", "model", "org", "space", "user"}, t) || n == "" {
			return nil, "invalid watched item"
		}
		out = append(out, map[string]any{"type": t, "name": n})
	}
	return out, ""
}

func validDomains(raw any) ([]string, string) {
	items, _ := raw.([]any)
	var out []string
	for _, it := range items {
		s, _ := it.(string)
		if s != "repo" && s != "discussion" {
			return nil, "invalid domain"
		}
		out = append(out, s)
	}
	return out, ""
}

func (f *fakeHF) handleWebhook(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			apiError(w, 400, "expected a JSON object")
			return
		}
	}
	if len(parts) == 1 && parts[0] == "" && r.Method == "POST" {
		watched, msg := validWatched(body["watched"])
		if msg != "" {
			apiError(w, 400, msg)
			return
		}
		domains, msg := validDomains(body["domains"])
		if msg != "" || len(domains) == 0 {
			apiError(w, 400, "domains are required")
			return
		}
		url, _ := body["url"].(string)
		if _, hasJob := body["jobSourceId"]; url == "" && !hasJob {
			apiError(w, 400, "url or jobSourceId is required")
			return
		}
		wh := &fakeWebhook{id: f.newID("wh"), url: url, watched: watched, domains: domains}
		wh.secret, _ = body["secret"].(string)
		f.webhooks[wh.id] = wh
		writeJSON(w, 200, wh.view())
		return
	}
	if len(parts) == 1 && parts[0] == "" && r.Method == "GET" {
		list := []any{}
		for _, id := range slices.Sorted(mapKeys(f.webhooks)) {
			list = append(list, f.webhooks[id].view()["webhook"])
		}
		writeJSON(w, 200, map[string]any{"webhooks": list})
		return
	}
	wh, ok := f.webhooks[parts[0]]
	if !ok {
		apiError(w, 404, "Webhook not found")
		return
	}
	switch {
	case len(parts) == 1 && r.Method == "GET":
		writeJSON(w, 200, wh.view())
	case len(parts) == 1 && r.Method == "DELETE":
		delete(f.webhooks, wh.id)
		w.WriteHeader(200)
	case len(parts) == 1 && r.Method == "POST":
		// Updates always carry the full watched list.
		watched, msg := validWatched(body["watched"])
		if msg != "" {
			apiError(w, 400, msg)
			return
		}
		wh.watched = watched
		if u, ok := body["url"].(string); ok {
			wh.url = u
		}
		if _, ok := body["domains"]; ok {
			domains, msg := validDomains(body["domains"])
			if msg != "" || len(domains) == 0 {
				apiError(w, 400, "invalid domains")
				return
			}
			wh.domains = domains
		}
		if s, ok := body["secret"].(string); ok {
			wh.secret = s
		}
		writeJSON(w, 200, wh.view())
	case len(parts) == 2 && parts[1] == "enable" && r.Method == "POST":
		wh.disabled = false
		writeJSON(w, 200, wh.view())
	case len(parts) == 2 && parts[1] == "disable" && r.Method == "POST":
		wh.disabled = true
		writeJSON(w, 200, wh.view())
	default:
		apiError(w, 404, "not found")
	}
}

func mapKeys[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

type fakeItem struct {
	oid, id, typ string
	note         *string
	position     int
}

type fakeCollection struct {
	slug, title, ns, description, theme string
	private                             bool
	items                               []*fakeItem
}

func (c *fakeCollection) view() map[string]any {
	items := []any{}
	for i, it := range c.items {
		m := map[string]any{"_id": it.oid, "id": it.id, "type": it.typ, "position": i}
		if it.typ == "collection" {
			m["slug"] = it.id
		}
		if it.note != nil {
			m["note"] = map[string]any{"text": *it.note}
		}
		items = append(items, m)
	}
	out := map[string]any{
		"slug": c.slug, "title": c.title, "owner": map[string]any{"name": c.ns}, "items": items,
		"lastUpdated": "2026-01-01T00:00:00.000Z", "position": 0, "private": c.private, "theme": c.theme, "upvotes": 0,
	}
	if c.description != "" {
		out["description"] = c.description
	}
	return out
}

func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (f *fakeHF) handleCollection(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			apiError(w, 400, "expected a JSON object")
			return
		}
	}
	if len(parts) == 1 && parts[0] == "" && r.Method == "POST" {
		title, _ := body["title"].(string)
		ns, _ := body["namespace"].(string)
		if title == "" || ns == "" {
			apiError(w, 400, "title and namespace are required")
			return
		}
		c := &fakeCollection{title: title, ns: ns, theme: "orange"}
		c.slug = ns + "/" + slugify(title) + "-" + f.newID("c")
		c.private, _ = body["private"].(bool)
		c.description, _ = body["description"].(string)
		f.collections[c.slug] = c
		writeJSON(w, 200, c.view())
		return
	}
	if len(parts) < 2 {
		apiError(w, 404, "not found")
		return
	}
	c, ok := f.collections[parts[0]+"/"+parts[1]]
	if !ok {
		apiError(w, 404, "Collection not found")
		return
	}
	rest := parts[2:]
	switch {
	case len(rest) == 0 && r.Method == "GET":
		writeJSON(w, 200, c.view())
	case len(rest) == 0 && r.Method == "DELETE":
		delete(f.collections, c.slug)
		w.WriteHeader(200)
	case len(rest) == 0 && r.Method == "PATCH":
		for k, v := range body {
			switch k {
			case "title":
				c.title, _ = v.(string)
			case "description":
				c.description, _ = v.(string)
			case "private":
				c.private, _ = v.(bool)
			case "theme":
				c.theme, _ = v.(string)
			case "position":
			default:
				apiError(w, 400, "unknown field "+k)
				return
			}
		}
		writeJSON(w, 200, map[string]any{"data": c.view()})
	case len(rest) == 1 && rest[0] == "items" && r.Method == "POST":
		item := asMap(body["item"])
		id, _ := item["id"].(string)
		typ, _ := item["type"].(string)
		if id == "" || !slices.Contains([]string{"model", "dataset", "space", "paper", "collection", "bucket"}, typ) {
			apiError(w, 400, "invalid item")
			return
		}
		for _, it := range c.items {
			if it.id == id && it.typ == typ {
				apiError(w, 409, "Item already exists in collection")
				return
			}
		}
		it := &fakeItem{oid: f.newID("i"), id: id, typ: typ}
		if n, ok := body["note"].(string); ok {
			it.note = &n
		}
		c.items = append(c.items, it)
		writeJSON(w, 200, c.view())
	case len(rest) == 2 && rest[0] == "items":
		idx := slices.IndexFunc(c.items, func(it *fakeItem) bool { return it.oid == rest[1] })
		if idx < 0 {
			apiError(w, 404, "Item not found")
			return
		}
		switch r.Method {
		case "PATCH":
			if n, ok := body["note"].(string); ok {
				c.items[idx].note = &n
			}
			w.WriteHeader(200)
		case "DELETE":
			c.items = slices.Delete(c.items, idx, idx+1)
			w.WriteHeader(200)
		default:
			apiError(w, 405, "method not allowed")
		}
	default:
		apiError(w, 404, "not found")
	}
}

// ---------------------------------------------------------------------------
// Git refs
// ---------------------------------------------------------------------------

// resolveRevision maps a branch, tag or commit SHA to a commit.
func (repo *fakeRepo) resolveRevision(rev string) (string, bool) {
	if c, ok := repo.branches[rev]; ok {
		return c, true
	}
	if c, ok := repo.tags[rev]; ok {
		return c, true
	}
	for _, c := range repo.branches {
		if c == rev {
			return c, true
		}
	}
	return "", false
}

// tagsAsListed is what /refs reports for tags: the tag object for an annotated
// tag, the commit for a lightweight one.
func (repo *fakeRepo) tagsAsListed() map[string]string {
	out := map[string]string{}
	for k, v := range repo.tags {
		if obj, ok := repo.tagObjects[k]; ok {
			v = obj
		}
		out[k] = v
	}
	return out
}

func refsView(prefix string, m map[string]string) []any {
	out := []any{}
	for _, name := range slices.Sorted(mapKeys(m)) {
		out = append(out, map[string]any{"name": name, "ref": prefix + name, "targetCommit": m[name]})
	}
	return out
}

func (f *fakeHF) handleRefs(w http.ResponseWriter, r *http.Request, repo *fakeRepo, sub string, rest []string, body map[string]any) {
	name := strings.Join(rest, "/")
	switch {
	case sub == "refs" && r.Method == "GET":
		writeJSON(w, 200, map[string]any{
			"branches": refsView("refs/heads/", repo.branches), "converts": []any{},
			"tags": refsView("refs/tags/", repo.tagsAsListed()), "pullRequests": []any{},
		})
	case sub == "revision" && r.Method == "GET":
		commit, found := repo.resolveRevision(name)
		if !found {
			apiError(w, 404, "Revision not found")
			return
		}
		writeJSON(w, 200, map[string]any{"sha": commit})
	case sub == "branch" && r.Method == "POST":
		if _, exists := repo.branches[name]; exists {
			apiError(w, 409, "Branch already exists")
			return
		}
		commit := repo.branches["main"]
		if sp, ok := body["startingPoint"].(string); ok {
			c, found := repo.resolveRevision(sp)
			if !found {
				apiError(w, 404, "Revision not found")
				return
			}
			commit = c
		}
		repo.branches[name] = commit
		w.WriteHeader(200)
	case sub == "branch" && r.Method == "DELETE":
		if _, ok := repo.branches[name]; !ok || name == "main" {
			apiError(w, 404, "Branch not found")
			return
		}
		delete(repo.branches, name)
		w.WriteHeader(200)
	case sub == "tag" && r.Method == "POST":
		tag, _ := body["tag"].(string)
		if tag == "" {
			apiError(w, 400, "tag is required")
			return
		}
		if _, exists := repo.tags[tag]; exists {
			apiError(w, 409, "Tag already exists")
			return
		}
		commit, found := repo.resolveRevision(name)
		if !found {
			apiError(w, 404, "Revision not found")
			return
		}
		repo.tags[tag] = commit
		// Like the real Hub, a tag with a message is annotated: /refs reports the
		// tag object, not the commit, while /revision peels it.
		if msg, _ := body["message"].(string); msg != "" {
			repo.tagObjects[tag] = "tagobj-" + commit
		}
		w.WriteHeader(200)
	case sub == "tag" && r.Method == "DELETE":
		if _, ok := repo.tags[name]; !ok {
			apiError(w, 404, "Tag not found")
			return
		}
		delete(repo.tags, name)
		delete(repo.tagObjects, name)
		w.WriteHeader(200)
	default:
		apiError(w, 405, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Buckets
// ---------------------------------------------------------------------------

type fakeBucket struct {
	private  bool
	region   string
	rgroup   string
	createdA string
}

func (f *fakeHF) handleBucket(w http.ResponseWriter, r *http.Request, parts []string, raw []byte) {
	if len(parts) != 2 {
		apiError(w, 404, "not found")
		return
	}
	var body map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			apiError(w, 400, "expected a JSON object")
			return
		}
	}
	key := parts[0] + "/" + parts[1]
	b, exists := f.buckets[key]
	switch r.Method {
	case "POST":
		if exists {
			apiError(w, 409, "Bucket already exists")
			return
		}
		nb := &fakeBucket{createdA: "2026-01-01T00:00:00.000Z"}
		if v, ok := body["visibility"].(string); ok {
			if v != "public" && v != "private" {
				apiError(w, 400, "invalid visibility")
				return
			}
			nb.private = v == "private"
		}
		nb.region, _ = body["region"].(string)
		nb.rgroup, _ = body["resourceGroupId"].(string)
		f.buckets[key] = nb
		writeJSON(w, 200, map[string]any{"url": "https://huggingface.co/buckets/" + key, "name": key, "id": key})
	case "GET":
		if !exists {
			apiError(w, 404, "Bucket not found")
			return
		}
		writeJSON(w, 200, map[string]any{"id": key, "private": b.private, "createdAt": b.createdA, "size": 0, "totalFiles": 0})
	case "DELETE":
		if !exists {
			apiError(w, 404, "Bucket not found")
			return
		}
		delete(f.buckets, key)
		w.WriteHeader(200)
	default:
		apiError(w, 405, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Test helpers over the new state
// ---------------------------------------------------------------------------

func (f *fakeHF) webhook(id string) *fakeWebhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	if wh, ok := f.webhooks[id]; ok {
		cp := *wh
		return &cp
	}
	return nil
}

func (f *fakeHF) mutateWebhook(id string, fn func(*fakeWebhook)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.webhooks[id])
}

func (f *fakeHF) deleteWebhookOutOfBand(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.webhooks, id)
}

func (f *fakeHF) addWebhook(wh *fakeWebhook) {
	f.mu.Lock()
	defer f.mu.Unlock()
	wh.id = f.newID("wh")
	f.webhooks[wh.id] = wh
}

func (f *fakeHF) collection(slug string) *fakeCollection {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.collections[slug]; ok {
		cp := *c
		cp.items = slices.Clone(c.items)
		return &cp
	}
	return nil
}

func (f *fakeHF) mutateCollection(slug string, fn func(*fakeCollection)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.collections[slug])
}

func (f *fakeHF) deleteCollectionOutOfBand(slug string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.collections, slug)
}

func (f *fakeHF) addCollection(ns, title string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &fakeCollection{title: title, ns: ns, theme: "blue"}
	c.slug = ns + "/" + slugify(title) + "-" + f.newID("c")
	f.collections[c.slug] = c
	return c.slug
}

func (f *fakeHF) addCollectionItem(slug, id, typ string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	it := &fakeItem{oid: f.newID("i"), id: id, typ: typ}
	f.collections[slug].items = append(f.collections[slug].items, it)
	return it.oid
}

func (f *fakeHF) bucket(key string) *fakeBucket {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.buckets[key]; ok {
		cp := *b
		return &cp
	}
	return nil
}

func (f *fakeHF) mutateBucket(key string, fn func(*fakeBucket)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.buckets[key])
}

func (f *fakeHF) deleteBucketOutOfBand(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.buckets, key)
}

func (f *fakeHF) addBucket(key string, private bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buckets[key] = &fakeBucket{private: private, createdA: "2026-01-01T00:00:00.000Z"}
}

// refs returns copies of a repo's branches and tags.
func (f *fakeHF) refs(typ, ns, name string) (branches, tags map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.repos[repoKey(typ, ns, name)]
	if !ok {
		return nil, nil
	}
	branches, tags = map[string]string{}, map[string]string{}
	for k, v := range r.branches {
		branches[k] = v
	}
	for k, v := range r.tags {
		tags[k] = v
	}
	return branches, tags
}

func (f *fakeHF) mutateRefs(typ, ns, name string, fn func(branches, tags map[string]string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.repos[repoKey(typ, ns, name)]
	fn(r.branches, r.tags)
}
