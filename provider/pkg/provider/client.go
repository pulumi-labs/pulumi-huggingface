// Package provider implements the Hugging Face Pulumi provider resources.
//
// Two API planes are covered:
//   - Inference Endpoints API (api.endpoints.huggingface.cloud)
//   - Hub API (huggingface.co/api) — repositories, Spaces, webhooks
package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Base URLs of the two API planes. They are variables so tests can point the
// client at an in-memory fake server.
var (
	endpointsBase = "https://api.endpoints.huggingface.cloud/v2"
	hubBase       = "https://huggingface.co"
)

// hfClient is a minimal HTTP client for the Hugging Face APIs.
type hfClient struct {
	token      string
	httpClient *http.Client
}

func newHFClient(token string) *hfClient {
	return &hfClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *hfClient) do(method, url string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return respBody, resp.StatusCode, fmt.Errorf("API error %d %s: %s", resp.StatusCode, url, string(respBody))
	}
	return respBody, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Inference Endpoints API types and operations
// ---------------------------------------------------------------------------

// EndpointType controls access to the inference endpoint. The Inference
// Endpoints API accepts public, authenticated and private; the legacy
// "protected" value has been removed.
type EndpointType string

const (
	EndpointTypePublic        EndpointType = "public"
	EndpointTypeAuthenticated EndpointType = "authenticated"
	EndpointTypePrivate       EndpointType = "private"
)

// EndpointState represents the lifecycle state of an inference endpoint.
type EndpointState string

const (
	EndpointStatePending      EndpointState = "pending"
	EndpointStateInitializing EndpointState = "initializing"
	EndpointStateUpdating     EndpointState = "updating"
	EndpointStateUpdateFailed EndpointState = "updateFailed"
	EndpointStateRunning      EndpointState = "running"
	EndpointStatePaused       EndpointState = "paused"
	EndpointStateFailed       EndpointState = "failed"
	EndpointStateScaledToZero EndpointState = "scaledToZero"
)

// EndpointScalingMeasure controls the autoscaling metric.
type EndpointScalingMeasure struct {
	HardwareUsage   *float64 `json:"hardwareUsage,omitempty"`
	PendingRequests *float64 `json:"pendingRequests,omitempty"`
}

// EndpointScaling defines the replica autoscaling configuration.
type EndpointScaling struct {
	MinReplica         int                     `json:"minReplica"`
	MaxReplica         int                     `json:"maxReplica"`
	ScaleToZeroTimeout *int                    `json:"scaleToZeroTimeout,omitempty"`
	Measure            *EndpointScalingMeasure `json:"measure,omitempty"`
}

// EndpointCompute defines hardware and scaling for an endpoint.
type EndpointCompute struct {
	Accelerator  string          `json:"accelerator"`
	InstanceType string          `json:"instanceType"`
	InstanceSize string          `json:"instanceSize"`
	Scaling      EndpointScaling `json:"scaling"`
}

// EndpointCustomImage is the "custom" container variant of model.image.
type EndpointCustomImage struct {
	URL         string            `json:"url"`
	Credentials *ImageCredentials `json:"credentials,omitempty"`
	HealthRoute *string           `json:"healthRoute,omitempty"`
	Port        *int              `json:"port,omitempty"`
}

// ImageCredentials holds credentials for a private container registry.
type ImageCredentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

// EndpointImage is the model.image union. The API requires exactly one
// variant: {"huggingface": {}} for the stock Hugging Face image, or
// {"custom": {...}} for a user-supplied container. Other variants (tgi, vLLM,
// ...) exist; they are ignored when read back.
type EndpointImage struct {
	Huggingface *struct{}            `json:"huggingface,omitempty"`
	Custom      *EndpointCustomImage `json:"custom,omitempty"`
}

// EndpointModel describes the model to serve.
type EndpointModel struct {
	Repository string             `json:"repository,omitempty"`
	Framework  string             `json:"framework,omitempty"`
	Task       string             `json:"task,omitempty"`
	Revision   *string            `json:"revision,omitempty"`
	Image      *EndpointImage     `json:"image,omitempty"`
	Env        *map[string]string `json:"env,omitempty"`
}

// EndpointProvider specifies the cloud vendor and region.
type EndpointProvider struct {
	Vendor string `json:"vendor,omitempty"`
	Region string `json:"region,omitempty"`
}

// PrivateService configures AWS PrivateLink for a private endpoint.
type PrivateService struct {
	AccountID string `json:"accountId,omitempty"`
	Region    string `json:"region,omitempty"`
	Shared    bool   `json:"shared,omitempty"`
}

// EndpointStatus is the read-only runtime status of an endpoint.
type EndpointStatus struct {
	State   EndpointState `json:"state"`
	Message string        `json:"message"`
	URL     string        `json:"url"`
}

// EndpointDetails is the full endpoint representation returned by the API.
type EndpointDetails struct {
	Name           string           `json:"name"`
	Type           EndpointType     `json:"type"`
	Compute        EndpointCompute  `json:"compute"`
	Model          EndpointModel    `json:"model"`
	Provider       EndpointProvider `json:"provider"`
	PrivateService *PrivateService  `json:"privateService,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	Status         EndpointStatus   `json:"status"`
}

// CreateEndpointRequest is the payload for POST /endpoint/{namespace}.
type CreateEndpointRequest struct {
	Name           string           `json:"name"`
	Type           EndpointType     `json:"type"`
	Compute        EndpointCompute  `json:"compute"`
	Model          EndpointModel    `json:"model"`
	Provider       EndpointProvider `json:"provider"`
	PrivateService *PrivateService  `json:"privateService,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
}

// UpdateEndpointRequest is the payload for PUT /endpoint/{namespace}/{name}.
type UpdateEndpointRequest struct {
	Type     *EndpointType     `json:"type,omitempty"`
	Compute  *EndpointCompute  `json:"compute,omitempty"`
	Model    *EndpointModel    `json:"model,omitempty"`
	Provider *EndpointProvider `json:"provider,omitempty"`
	// Tags is a pointer so an empty list ([]) clears the tags.
	Tags *[]string `json:"tags,omitempty"`
}

func decodeEndpoint(body []byte) (*EndpointDetails, error) {
	var out EndpointDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal endpoint: %w", err)
	}
	return &out, nil
}

func (c *hfClient) CreateEndpoint(namespace string, req CreateEndpointRequest) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s", endpointsBase, namespace)
	body, _, err := c.do("POST", url, req)
	if err != nil {
		return nil, fmt.Errorf("create endpoint: %w", err)
	}
	return decodeEndpoint(body)
}

// GetEndpoint returns nil, nil when the endpoint does not exist.
func (c *hfClient) GetEndpoint(namespace, name string) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	body, statusCode, err := c.do("GET", url, nil)
	if err != nil {
		if statusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	return decodeEndpoint(body)
}

func (c *hfClient) UpdateEndpoint(namespace, name string, req UpdateEndpointRequest) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	body, _, err := c.do("PUT", url, req)
	if err != nil {
		return nil, fmt.Errorf("update endpoint: %w", err)
	}
	return decodeEndpoint(body)
}

// DeleteEndpoint is idempotent: an endpoint that is already gone is not an error.
func (c *hfClient) DeleteEndpoint(namespace, name string) error {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	_, statusCode, err := c.do("DELETE", url, nil)
	if err != nil {
		if statusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("delete endpoint: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Hub API types and operations: repositories and Spaces
// ---------------------------------------------------------------------------

// SpaceSDK is the frontend framework for a Space.
type SpaceSDK string

const (
	SpaceSDKGradio    SpaceSDK = "gradio"
	SpaceSDKStreamlit SpaceSDK = "streamlit"
	SpaceSDKDocker    SpaceSDK = "docker"
	SpaceSDKStatic    SpaceSDK = "static"
)

// SpaceHardware is the compute tier for a Space. The Hub owns this enum and
// adds flavors over time (h200, l4x*, l40sx*, a100x*, rtx-pro-6000*, ...), so
// the provider does not validate it beyond rejecting the retired h100 names.
type SpaceHardware string

const (
	SpaceHardwareCPUBasic   SpaceHardware = "cpu-basic"
	SpaceHardwareCPUUpgrade SpaceHardware = "cpu-upgrade"
	SpaceHardwareT4Small    SpaceHardware = "t4-small"
	SpaceHardwareT4Medium   SpaceHardware = "t4-medium"
	SpaceHardwareL4x1       SpaceHardware = "l4x1"
	SpaceHardwareL4x4       SpaceHardware = "l4x4"
	SpaceHardwareL40Sx1     SpaceHardware = "l40sx1"
	SpaceHardwareA10GSmall  SpaceHardware = "a10g-small"
	SpaceHardwareA10GLarge  SpaceHardware = "a10g-large"
	SpaceHardwareA100Large  SpaceHardware = "a100-large"
	SpaceHardwareA100x4     SpaceHardware = "a100x4"
	SpaceHardwareA100x8     SpaceHardware = "a100x8"
)

// SpaceEnvVar is a non-secret environment variable for a Space.
type SpaceEnvVar struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// SpaceSecret is a secret environment variable for a Space.
type SpaceSecret struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// CreateRepoRequest is the payload for POST /api/repos/create.
type CreateRepoRequest struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"` // "model" | "dataset" | "space"
	Organization *string       `json:"organization,omitempty"`
	Private      *bool         `json:"private,omitempty"`
	SDK          *string       `json:"sdk,omitempty"`
	Hardware     *string       `json:"hardware,omitempty"`
	SleepTime    *int          `json:"sleepTimeSeconds,omitempty"`
	Secrets      []SpaceSecret `json:"secrets,omitempty"`
	Variables    []SpaceEnvVar `json:"variables,omitempty"`
}

// SpaceRuntime is the runtime block of a Space (hardware is nested under
// runtime.hardware, not at the top level).
type SpaceRuntime struct {
	Stage    string `json:"stage"`
	Hardware *struct {
		Current   *string `json:"current"`
		Requested *string `json:"requested"`
	} `json:"hardware"`
	GCTimeout *int `json:"gcTimeout"`
}

// RepoDetails is the response from the Hub API for a repository.
type RepoDetails struct {
	ID      string        `json:"id"` // "namespace/name"
	Name    string        `json:"name"`
	Private bool          `json:"private"`
	SDK     *string       `json:"sdk,omitempty"`
	Runtime *SpaceRuntime `json:"runtime,omitempty"`
	// Gated is false, "auto" or "manual" (models and datasets).
	Gated json.RawMessage `json:"gated,omitempty"`
}

// EffectiveHardware returns the hardware the Space is configured for: the
// requested flavor if a request is pending or recorded, else the current one.
func (r *RepoDetails) EffectiveHardware() (string, bool) {
	if r.Runtime == nil || r.Runtime.Hardware == nil {
		return "", false
	}
	if h := r.Runtime.Hardware.Requested; h != nil && *h != "" {
		return *h, true
	}
	if h := r.Runtime.Hardware.Current; h != nil && *h != "" {
		return *h, true
	}
	return "", false
}

func repoAPIPath(repoType string) string {
	switch repoType {
	case "space":
		return "spaces"
	case "dataset":
		return "datasets"
	default:
		return "models"
	}
}

func (c *hfClient) CreateRepo(req CreateRepoRequest) (*RepoDetails, error) {
	url := fmt.Sprintf("%s/api/repos/create", hubBase)
	body, _, err := c.do("POST", url, req)
	if err != nil {
		return nil, fmt.Errorf("create repo: %w", err)
	}
	var out RepoDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal repo: %w", err)
	}
	return &out, nil
}

// GetRepo returns nil, nil when the repository does not exist.
func (c *hfClient) GetRepo(repoType, namespace, name string) (*RepoDetails, error) {
	url := fmt.Sprintf("%s/api/%s/%s/%s", hubBase, repoAPIPath(repoType), namespace, name)
	body, statusCode, err := c.do("GET", url, nil)
	if err != nil {
		if statusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get repo: %w", err)
	}
	var out RepoDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal repo: %w", err)
	}
	return &out, nil
}

// DeleteRepo is idempotent: a repository that is already gone is not an error.
func (c *hfClient) DeleteRepo(repoType, namespace, name string) error {
	url := fmt.Sprintf("%s/api/repos/delete", hubBase)
	payload := map[string]string{
		"name": name,
		"type": repoType,
	}
	if namespace != "" {
		payload["organization"] = namespace
	}
	_, statusCode, err := c.do("DELETE", url, payload)
	if err != nil {
		if statusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("delete repo: %w", err)
	}
	return nil
}

// UpdateRepoVisibility changes a repository's visibility through
// PUT /api/{type}s/{namespace}/{name}/settings.
func (c *hfClient) UpdateRepoVisibility(repoType, namespace, name string, private bool) error {
	url := fmt.Sprintf("%s/api/%s/%s/%s/settings", hubBase, repoAPIPath(repoType), namespace, name)
	_, _, err := c.do("PUT", url, map[string]bool{"private": private})
	if err != nil {
		return fmt.Errorf("update %s visibility: %w", repoType, err)
	}
	return nil
}

// UpdateSpaceHardware changes the hardware tier for a Space.
func (c *hfClient) UpdateSpaceHardware(namespace, name string, hardware SpaceHardware) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/hardware", hubBase, namespace, name)
	_, _, err := c.do("POST", url, map[string]string{"flavor": string(hardware)})
	if err != nil {
		return fmt.Errorf("update space hardware: %w", err)
	}
	return nil
}

// UpdateSpaceSleepTime sets the idle time (seconds) before the Space sleeps.
func (c *hfClient) UpdateSpaceSleepTime(namespace, name string, seconds int) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/sleeptime", hubBase, namespace, name)
	_, _, err := c.do("POST", url, map[string]int{"seconds": seconds})
	if err != nil {
		return fmt.Errorf("update space sleep time: %w", err)
	}
	return nil
}

// UpsertSpaceSecret adds or replaces one secret (one {key,value,description}
// object per call, as the Hub API expects).
func (c *hfClient) UpsertSpaceSecret(namespace, name string, s SpaceSecret) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/secrets", hubBase, namespace, name)
	if _, _, err := c.do("POST", url, s); err != nil {
		return fmt.Errorf("set space secret %q: %w", s.Key, err)
	}
	return nil
}

// DeleteSpaceSecret removes one secret; a missing key is not an error.
func (c *hfClient) DeleteSpaceSecret(namespace, name, key string) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/secrets", hubBase, namespace, name)
	if _, code, err := c.do("DELETE", url, map[string]string{"key": key}); err != nil && code != http.StatusNotFound {
		return fmt.Errorf("delete space secret %q: %w", key, err)
	}
	return nil
}

// UpsertSpaceVariable adds or replaces one variable.
func (c *hfClient) UpsertSpaceVariable(namespace, name string, v SpaceEnvVar) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/variables", hubBase, namespace, name)
	if _, _, err := c.do("POST", url, v); err != nil {
		return fmt.Errorf("set space variable %q: %w", v.Key, err)
	}
	return nil
}

// DeleteSpaceVariable removes one variable; a missing key is not an error.
func (c *hfClient) DeleteSpaceVariable(namespace, name, key string) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/variables", hubBase, namespace, name)
	if _, code, err := c.do("DELETE", url, map[string]string{"key": key}); err != nil && code != http.StatusNotFound {
		return fmt.Errorf("delete space variable %q: %w", key, err)
	}
	return nil
}

// GetSpaceVariables lists the Space's variables keyed by name.
func (c *hfClient) GetSpaceVariables(namespace, name string) (map[string]SpaceEnvVar, error) {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/variables", hubBase, namespace, name)
	body, _, err := c.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get space variables: %w", err)
	}
	var raw map[string]struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal space variables: %w", err)
	}
	out := make(map[string]SpaceEnvVar, len(raw))
	for k, v := range raw {
		out[k] = SpaceEnvVar{Key: k, Value: v.Value, Description: v.Description}
	}
	return out, nil
}

// GetSpaceSecretKeys lists the names of the Space's secrets. Values are
// write-only and never returned by the API.
func (c *hfClient) GetSpaceSecretKeys(namespace, name string) (map[string]bool, error) {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/secrets", hubBase, namespace, name)
	body, _, err := c.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get space secrets: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal space secrets: %w", err)
	}
	out := make(map[string]bool, len(raw))
	for k := range raw {
		out[k] = true
	}
	return out, nil
}
