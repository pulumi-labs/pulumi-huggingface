package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// This file holds the Hub API calls added with the coverage work: repository
// settings, webhooks, collections, git refs, buckets, whoami and endpoint
// listing. Request and response shapes follow huggingface_hub (hf_api.py).

func isNotFound(code int) bool { return code == http.StatusNotFound }

// ---------------------------------------------------------------------------
// whoami
// ---------------------------------------------------------------------------

// Whoami is the part of GET /api/whoami-v2 the provider uses.
type Whoami struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	FullName string `json:"fullname"`
	Orgs     []struct {
		Name string `json:"name"`
	} `json:"orgs"`
}

func (c *hfClient) Whoami() (*Whoami, error) {
	body, _, err := c.do("GET", hubBase+"/api/whoami-v2", nil)
	if err != nil {
		return nil, fmt.Errorf("whoami: %w", err)
	}
	var out Whoami
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal whoami: %w", err)
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Inference Endpoints: list
// ---------------------------------------------------------------------------

// ListEndpoints returns every endpoint of a namespace (GET /endpoint/{ns},
// the endpoints are under "items").
func (c *hfClient) ListEndpoints(namespace string) ([]EndpointDetails, error) {
	body, _, err := c.do("GET", fmt.Sprintf("%s/endpoint/%s", endpointsBase, namespace), nil)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}
	var out struct {
		Items []EndpointDetails `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal endpoints: %w", err)
	}
	return out.Items, nil
}

// ---------------------------------------------------------------------------
// Repository settings
// ---------------------------------------------------------------------------

// gatedValue maps the API's gated field (false, "auto" or "manual") onto the
// provider's "off", "auto", "manual".
func gatedValue(raw json.RawMessage) string {
	var s string
	if len(raw) > 0 && json.Unmarshal(raw, &s) == nil && (s == "auto" || s == "manual") {
		return s
	}
	return "off"
}

// UpdateRepoSettings sends PUT /api/{type}s/{ns}/{name}/settings with the
// given settings ("private" and/or "gated": "auto"|"manual"|false).
func (c *hfClient) UpdateRepoSettings(repoType, namespace, name string, settings map[string]any) error {
	u := fmt.Sprintf("%s/api/%s/%s/%s/settings", hubBase, repoAPIPath(repoType), namespace, name)
	if _, _, err := c.do("PUT", u, settings); err != nil {
		return fmt.Errorf("update %s settings: %w", repoType, err)
	}
	return nil
}

// gatedSetting converts "off"|"auto"|"manual" to the settings payload value.
func gatedSetting(v string) any {
	if v == "auto" || v == "manual" {
		return v
	}
	return false
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

// WebhookWatched is one watched item of a webhook.
type WebhookWatched struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// WebhookDetails is the webhook representation under "webhook" in responses.
type WebhookDetails struct {
	ID       string           `json:"id"`
	URL      *string          `json:"url"`
	Watched  []WebhookWatched `json:"watched"`
	Domains  []string         `json:"domains"`
	Disabled bool             `json:"disabled"`
}

func decodeWebhook(body []byte) (*WebhookDetails, error) {
	var env struct {
		Webhook WebhookDetails `json:"webhook"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("unmarshal webhook: %w", err)
	}
	return &env.Webhook, nil
}

func (c *hfClient) CreateWebhook(payload map[string]any) (*WebhookDetails, error) {
	body, _, err := c.do("POST", hubBase+"/api/settings/webhooks", payload)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	return decodeWebhook(body)
}

// GetWebhook returns nil, nil when the webhook does not exist.
func (c *hfClient) GetWebhook(id string) (*WebhookDetails, error) {
	body, code, err := c.do("GET", hubBase+"/api/settings/webhooks/"+url.PathEscape(id), nil)
	if err != nil {
		if isNotFound(code) {
			return nil, nil
		}
		return nil, fmt.Errorf("get webhook: %w", err)
	}
	return decodeWebhook(body)
}

func (c *hfClient) UpdateWebhook(id string, payload map[string]any) (*WebhookDetails, error) {
	body, _, err := c.do("POST", hubBase+"/api/settings/webhooks/"+url.PathEscape(id), payload)
	if err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	return decodeWebhook(body)
}

// SetWebhookEnabled calls POST .../{id}/enable or .../{id}/disable.
func (c *hfClient) SetWebhookEnabled(id string, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	if _, _, err := c.do("POST", hubBase+"/api/settings/webhooks/"+url.PathEscape(id)+"/"+action, nil); err != nil {
		return fmt.Errorf("%s webhook: %w", action, err)
	}
	return nil
}

// DeleteWebhook is idempotent.
func (c *hfClient) DeleteWebhook(id string) error {
	_, code, err := c.do("DELETE", hubBase+"/api/settings/webhooks/"+url.PathEscape(id), nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete webhook: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

// CollectionItemDetails is an item of a collection. Collection items carry a
// slug instead of an id when they are collections themselves.
type CollectionItemDetails struct {
	ObjectID string `json:"_id"`
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Type     string `json:"type"`
	Position int    `json:"position"`
	Note     *struct {
		Text string `json:"text"`
	} `json:"note"`
}

// ItemID is the repo ID, paper ID or (for collections) slug of the item.
func (i CollectionItemDetails) ItemID() string {
	if i.Slug != "" {
		return i.Slug
	}
	return i.ID
}

// NoteText returns the note and whether one is set.
func (i CollectionItemDetails) NoteText() (string, bool) {
	if i.Note == nil {
		return "", false
	}
	return i.Note.Text, true
}

// CollectionDetails is a Hub collection.
type CollectionDetails struct {
	Slug        string                  `json:"slug"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Private     bool                    `json:"private"`
	Theme       string                  `json:"theme"`
	Position    int                     `json:"position"`
	Items       []CollectionItemDetails `json:"items"`
}

func decodeCollection(body []byte) (*CollectionDetails, error) {
	var out CollectionDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal collection: %w", err)
	}
	return &out, nil
}

func (c *hfClient) CreateCollection(payload map[string]any) (*CollectionDetails, error) {
	body, _, err := c.do("POST", hubBase+"/api/collections", payload)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	return decodeCollection(body)
}

// GetCollection returns nil, nil when the collection does not exist. The slug
// contains a slash that must stay a path separator.
func (c *hfClient) GetCollection(slug string) (*CollectionDetails, error) {
	body, code, err := c.do("GET", hubBase+"/api/collections/"+slug, nil)
	if err != nil {
		if isNotFound(code) {
			return nil, nil
		}
		return nil, fmt.Errorf("get collection: %w", err)
	}
	return decodeCollection(body)
}

// UpdateCollection patches title, description, private and theme.
func (c *hfClient) UpdateCollection(slug string, patch map[string]any) error {
	if _, _, err := c.do("PATCH", hubBase+"/api/collections/"+slug, patch); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	return nil
}

func (c *hfClient) DeleteCollection(slug string) error {
	_, code, err := c.do("DELETE", hubBase+"/api/collections/"+slug, nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

// AddCollectionItem posts one item and returns the updated collection.
func (c *hfClient) AddCollectionItem(slug, itemID, itemType string, note *string) (*CollectionDetails, error) {
	payload := map[string]any{"item": map[string]any{"id": itemID, "type": itemType}}
	if note != nil {
		payload["note"] = *note
	}
	body, _, err := c.do("POST", hubBase+"/api/collections/"+slug+"/items", payload)
	if err != nil {
		return nil, fmt.Errorf("add collection item: %w", err)
	}
	return decodeCollection(body)
}

func (c *hfClient) UpdateCollectionItem(slug, objectID string, patch map[string]any) error {
	if _, _, err := c.do("PATCH", hubBase+"/api/collections/"+slug+"/items/"+url.PathEscape(objectID), patch); err != nil {
		return fmt.Errorf("update collection item: %w", err)
	}
	return nil
}

func (c *hfClient) DeleteCollectionItem(slug, objectID string) error {
	_, code, err := c.do("DELETE", hubBase+"/api/collections/"+slug+"/items/"+url.PathEscape(objectID), nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete collection item: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Git refs: branches and tags
// ---------------------------------------------------------------------------

// GitRef is one entry of GET /api/{type}s/{id}/refs.
type GitRef struct {
	Name         string `json:"name"`
	Ref          string `json:"ref"`
	TargetCommit string `json:"targetCommit"`
}

// GitRefs is the refs response (pull requests and converts are not used).
type GitRefs struct {
	Branches []GitRef `json:"branches"`
	Tags     []GitRef `json:"tags"`
}

// GetRefs returns nil, nil when the repository does not exist.
func (c *hfClient) GetRefs(repoType, repoID string) (*GitRefs, error) {
	body, code, err := c.do("GET", fmt.Sprintf("%s/api/%s/%s/refs", hubBase, repoAPIPath(repoType), repoID), nil)
	if err != nil {
		if isNotFound(code) {
			return nil, nil
		}
		return nil, fmt.Errorf("list refs: %w", err)
	}
	var out GitRefs
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal refs: %w", err)
	}
	return &out, nil
}

// CreateBranch posts .../branch/{branch}; the branch name is path-escaped (a
// slash becomes %2F) and startingPoint is optional.
func (c *hfClient) CreateBranch(repoType, repoID, branch string, startingPoint *string) error {
	payload := map[string]any{}
	if startingPoint != nil {
		payload["startingPoint"] = *startingPoint
	}
	u := fmt.Sprintf("%s/api/%s/%s/branch/%s", hubBase, repoAPIPath(repoType), repoID, url.PathEscape(branch))
	if _, _, err := c.do("POST", u, payload); err != nil {
		return fmt.Errorf("create branch %q: %w", branch, err)
	}
	return nil
}

func (c *hfClient) DeleteBranch(repoType, repoID, branch string) error {
	u := fmt.Sprintf("%s/api/%s/%s/branch/%s", hubBase, repoAPIPath(repoType), repoID, url.PathEscape(branch))
	_, code, err := c.do("DELETE", u, nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete branch %q: %w", branch, err)
	}
	return nil
}

// CreateTag posts .../tag/{revision} with {"tag", "message"?}.
func (c *hfClient) CreateTag(repoType, repoID, tag, revision string, message *string) error {
	payload := map[string]any{"tag": tag}
	if message != nil {
		payload["message"] = *message
	}
	u := fmt.Sprintf("%s/api/%s/%s/tag/%s", hubBase, repoAPIPath(repoType), repoID, url.PathEscape(revision))
	if _, _, err := c.do("POST", u, payload); err != nil {
		return fmt.Errorf("create tag %q: %w", tag, err)
	}
	return nil
}

func (c *hfClient) DeleteTag(repoType, repoID, tag string) error {
	u := fmt.Sprintf("%s/api/%s/%s/tag/%s", hubBase, repoAPIPath(repoType), repoID, url.PathEscape(tag))
	_, code, err := c.do("DELETE", u, nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete tag %q: %w", tag, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Buckets
// ---------------------------------------------------------------------------

// BucketDetails is GET /api/buckets/{ns}/{name}.
type BucketDetails struct {
	ID         string `json:"id"`
	Private    bool   `json:"private"`
	CreatedAt  string `json:"createdAt"`
	Size       int64  `json:"size"`
	TotalFiles int64  `json:"totalFiles"`
}

// CreateBucket posts /api/buckets/{ns}/{name} with visibility, region and
// resourceGroupId when set.
func (c *hfClient) CreateBucket(namespace, name string, payload map[string]any) error {
	if _, _, err := c.do("POST", fmt.Sprintf("%s/api/buckets/%s/%s", hubBase, namespace, name), payload); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

// GetBucket returns nil, nil when the bucket does not exist.
func (c *hfClient) GetBucket(namespace, name string) (*BucketDetails, error) {
	body, code, err := c.do("GET", fmt.Sprintf("%s/api/buckets/%s/%s", hubBase, namespace, name), nil)
	if err != nil {
		if isNotFound(code) {
			return nil, nil
		}
		return nil, fmt.Errorf("get bucket: %w", err)
	}
	var out BucketDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal bucket: %w", err)
	}
	return &out, nil
}

func (c *hfClient) DeleteBucket(namespace, name string) error {
	_, code, err := c.do("DELETE", fmt.Sprintf("%s/api/buckets/%s/%s", hubBase, namespace, name), nil)
	if err != nil && !isNotFound(code) {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

// splitRefID splits "<type prefix>/<ns>/<name>@<ref>" into repo type, repo
// ID and ref. The prefix is models/, datasets/ or spaces/; no prefix means a
// model. The ref may contain slashes.
func splitRefID(id string) (repoType, repoID, ref string, err error) {
	at := strings.Index(id, "@")
	if at <= 0 || at == len(id)-1 {
		return "", "", "", fmt.Errorf("invalid ID %q: expected <type>/<namespace>/<name>@<ref>", id)
	}
	repoType, rest := "model", id[:at]
	for prefix, t := range map[string]string{"datasets/": "dataset", "models/": "model", "spaces/": "space"} {
		if strings.HasPrefix(rest, prefix) {
			repoType, rest = t, strings.TrimPrefix(rest, prefix)
			break
		}
	}
	if _, _, e := splitID(rest); e != nil || strings.Count(rest, "/") != 1 {
		return "", "", "", fmt.Errorf("invalid ID %q: expected <type>/<namespace>/<name>@<ref>", id)
	}
	return repoType, rest, id[at+1:], nil
}

func refPrefix(repoType string) string {
	switch repoType {
	case "dataset":
		return "datasets/"
	case "space":
		return "spaces/"
	default:
		return "models/"
	}
}
