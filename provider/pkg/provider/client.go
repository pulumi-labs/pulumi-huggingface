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

const (
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
		return nil, resp.StatusCode, fmt.Errorf("API error %d %s: %s", resp.StatusCode, url, string(respBody))
	}
	return respBody, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Inference Endpoints API types and operations
// ---------------------------------------------------------------------------

// EndpointType controls access to the inference endpoint.
type EndpointType string

const (
	EndpointTypePublic    EndpointType = "public"
	EndpointTypeProtected EndpointType = "protected"
	EndpointTypePrivate   EndpointType = "private"
)

// EndpointState represents the lifecycle state of an inference endpoint.
type EndpointState string

const (
	EndpointStatePending       EndpointState = "pending"
	EndpointStateInitializing  EndpointState = "initializing"
	EndpointStateUpdating      EndpointState = "updating"
	EndpointStateUpdateFailed  EndpointState = "updateFailed"
	EndpointStateRunning       EndpointState = "running"
	EndpointStatePaused        EndpointState = "paused"
	EndpointStateFailed        EndpointState = "failed"
	EndpointStateScaledToZero  EndpointState = "scaledToZero"
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

// EndpointCustomImage allows deploying a custom container image.
type EndpointCustomImage struct {
	URL         string            `json:"url"`
	Credentials *ImageCredentials `json:"credentials,omitempty"`
	Health      *ImageHealth      `json:"health,omitempty"`
	Port        *int              `json:"port,omitempty"`
	EnvVars     map[string]string `json:"env,omitempty"`
}

// ImageCredentials holds credentials for a private container registry.
type ImageCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ImageHealth configures the health check path for the container.
type ImageHealth struct {
	Route string `json:"route"`
}

// EndpointImage specifies the container configuration.
type EndpointImage struct {
	Custom *EndpointCustomImage `json:"custom,omitempty"`
}

// EndpointModel describes the model to serve.
type EndpointModel struct {
	Repository string         `json:"repository"`
	Framework  string         `json:"framework"`
	Task       string         `json:"task"`
	Revision   *string        `json:"revision,omitempty"`
	Image      *EndpointImage `json:"image,omitempty"`
}

// EndpointProvider specifies the cloud vendor and region.
type EndpointProvider struct {
	Vendor string `json:"vendor"`
	Region string `json:"region"`
}

// EndpointStatus is the read-only runtime status of an endpoint.
type EndpointStatus struct {
	State   EndpointState `json:"state"`
	Message string        `json:"message"`
	URL     string        `json:"url"`
}

// EndpointDetails is the full endpoint representation returned by the API.
type EndpointDetails struct {
	Name      string           `json:"name"`
	Namespace string           `json:"namespace"`
	Type      EndpointType     `json:"type"`
	AccountID *string          `json:"accountId,omitempty"`
	Compute   EndpointCompute  `json:"compute"`
	Model     EndpointModel    `json:"model"`
	Provider  EndpointProvider `json:"provider"`
	Status    EndpointStatus   `json:"status"`
}

// CreateEndpointRequest is the payload for POST /endpoint/{namespace}.
type CreateEndpointRequest struct {
	Name      string           `json:"name"`
	Type      EndpointType     `json:"type"`
	AccountID *string          `json:"accountId,omitempty"`
	Compute   EndpointCompute  `json:"compute"`
	Model     EndpointModel    `json:"model"`
	Provider  EndpointProvider `json:"provider"`
}

// UpdateEndpointRequest is the payload for PUT /endpoint/{namespace}/{name}.
type UpdateEndpointRequest struct {
	Type    *EndpointType    `json:"type,omitempty"`
	Compute *EndpointCompute `json:"compute,omitempty"`
	Model   *EndpointModel   `json:"model,omitempty"`
}

func (c *hfClient) CreateEndpoint(namespace string, req CreateEndpointRequest) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s", endpointsBase, namespace)
	body, _, err := c.do("POST", url, req)
	if err != nil {
		return nil, fmt.Errorf("create endpoint: %w", err)
	}
	var out EndpointDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal endpoint: %w", err)
	}
	return &out, nil
}

func (c *hfClient) GetEndpoint(namespace, name string) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	body, statusCode, err := c.do("GET", url, nil)
	if err != nil {
		if statusCode == 404 {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("get endpoint: %w", err)
	}
	var out EndpointDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal endpoint: %w", err)
	}
	return &out, nil
}

func (c *hfClient) UpdateEndpoint(namespace, name string, req UpdateEndpointRequest) (*EndpointDetails, error) {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	body, _, err := c.do("PUT", url, req)
	if err != nil {
		return nil, fmt.Errorf("update endpoint: %w", err)
	}
	var out EndpointDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal endpoint: %w", err)
	}
	return &out, nil
}

func (c *hfClient) DeleteEndpoint(namespace, name string) error {
	url := fmt.Sprintf("%s/endpoint/%s/%s", endpointsBase, namespace, name)
	_, _, err := c.do("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("delete endpoint: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Hub API types and operations — Spaces
// ---------------------------------------------------------------------------

// SpaceSDK is the frontend framework for a Space.
type SpaceSDK string

const (
	SpaceSDKGradio  SpaceSDK = "gradio"
	SpaceSDKDocker  SpaceSDK = "docker"
	SpaceSDKStatic  SpaceSDK = "static"
)

// SpaceHardware is the compute tier for a Space.
type SpaceHardware string

const (
	SpaceHardwareCPUBasic   SpaceHardware = "cpu-basic"
	SpaceHardwareCPUUpgrade SpaceHardware = "cpu-upgrade"
	SpaceHardwareT4Small    SpaceHardware = "t4-small"
	SpaceHardwareT4Medium   SpaceHardware = "t4-medium"
	SpaceHardwareA10GSmall  SpaceHardware = "a10g-small"
	SpaceHardwareA10GLarge  SpaceHardware = "a10g-large"
	SpaceHardwareA100Large  SpaceHardware = "a100-large"
	SpaceHardwareH100       SpaceHardware = "h100"
	SpaceHardwareH100x8     SpaceHardware = "h100x8"
)

// SpaceVisibility controls who can view the Space.
type SpaceVisibility string

const (
	SpaceVisibilityPublic    SpaceVisibility = "public"
	SpaceVisibilityPrivate   SpaceVisibility = "private"
	SpaceVisibilityProtected SpaceVisibility = "protected"
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
	Name         string          `json:"name"`
	Type         string          `json:"type"` // "model" | "dataset" | "space"
	Organization *string         `json:"organization,omitempty"`
	Private      *bool           `json:"private,omitempty"`
	Visibility   *string         `json:"visibility,omitempty"`
	SDK          *string         `json:"sdk,omitempty"`
	Hardware     *string         `json:"hardware,omitempty"`
	SleepTime    *int            `json:"sleepTimeSeconds,omitempty"`
	Secrets      []SpaceSecret   `json:"secrets,omitempty"`
	Variables    []SpaceEnvVar   `json:"variables,omitempty"`
}

// RepoDetails is the response from the Hub API for a repository.
type RepoDetails struct {
	ID           string  `json:"id"` // "namespace/name"
	Name         string  `json:"name"`
	Private      bool    `json:"private"`
	Author       string  `json:"author"`
	SHA          string  `json:"sha"`
	LastModified string  `json:"lastModified"`
	// Spaces-specific fields
	SDK      *string `json:"sdk,omitempty"`
	Hardware *string `json:"hardware,omitempty"`
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

func (c *hfClient) GetRepo(repoType, namespace, name string) (*RepoDetails, error) {
	var url string
	switch repoType {
	case "space":
		url = fmt.Sprintf("%s/api/spaces/%s/%s", hubBase, namespace, name)
	case "dataset":
		url = fmt.Sprintf("%s/api/datasets/%s/%s", hubBase, namespace, name)
	default:
		url = fmt.Sprintf("%s/api/models/%s/%s", hubBase, namespace, name)
	}
	body, statusCode, err := c.do("GET", url, nil)
	if err != nil {
		if statusCode == 404 {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("get repo: %w", err)
	}
	var out RepoDetails
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal repo: %w", err)
	}
	return &out, nil
}

func (c *hfClient) DeleteRepo(repoType, namespace, name string) error {
	url := fmt.Sprintf("%s/api/repos/delete", hubBase)
	payload := map[string]string{
		"name": name,
		"type": repoType,
	}
	if namespace != "" {
		payload["organization"] = namespace
	}
	_, _, err := c.do("DELETE", url, payload)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	return nil
}

// UpdateSpaceHardware changes the hardware tier for a Space.
func (c *hfClient) UpdateSpaceHardware(namespace, name string, hardware SpaceHardware) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/hardware", hubBase, namespace, name)
	payload := map[string]string{"flavor": string(hardware)}
	_, _, err := c.do("POST", url, payload)
	if err != nil {
		return fmt.Errorf("update space hardware: %w", err)
	}
	return nil
}

// UpdateSpaceSecrets replaces all secrets for a Space.
func (c *hfClient) UpdateSpaceSecrets(namespace, name string, secrets []SpaceSecret) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/secrets", hubBase, namespace, name)
	_, _, err := c.do("POST", url, secrets)
	if err != nil {
		return fmt.Errorf("update space secrets: %w", err)
	}
	return nil
}

// UpdateSpaceVariables replaces all non-secret environment variables for a Space.
func (c *hfClient) UpdateSpaceVariables(namespace, name string, vars []SpaceEnvVar) error {
	url := fmt.Sprintf("%s/api/spaces/%s/%s/variables", hubBase, namespace, name)
	_, _, err := c.do("POST", url, vars)
	if err != nil {
		return fmt.Errorf("update space variables: %w", err)
	}
	return nil
}
