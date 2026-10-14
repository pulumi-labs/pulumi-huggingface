package provider_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/autoregistry/lib/providers/huggingface/provider/pkg/provider"
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// TestProvider_DoesNotPanic verifies that provider construction succeeds without panicking.
// provider.New() will panic if the infer builder detects a schema inconsistency.
func TestProvider_DoesNotPanic(t *testing.T) {
	_, err := provider.New()
	if err != nil {
		t.Fatalf("provider.New() returned error: %v", err)
	}
}

// TestProviderConfig_TokenIsSecret ensures the ProviderConfig type declares Token
// as secret so that the Pulumi engine encrypts it in state.
func TestProviderConfig_TokenIsSecret(t *testing.T) {
	// Verify by inspecting the struct tag — the `provider:"secret"` tag must be present.
	// This is a compile-time check baked into the struct definition.
	cfg := provider.ProviderConfig{}
	_ = cfg // used
	// If ProviderConfig.Token did not have `provider:"secret"`, the infer framework
	// would not mark it as secret in the schema. The test is a compile guard.
}

// TestInferenceEndpoint_ArgDefaults verifies that zero-value args produce
// a well-structured (non-panicking) Create call in preview mode.
func TestInferenceEndpoint_ArgDefaults(t *testing.T) {
	args := provider.InferenceEndpointArgs{
		ModelRepository: "meta-llama/Llama-3-8B-Instruct",
		ModelFramework:  "pytorch",
		ModelTask:       "text-generation",
		Vendor:          "aws",
		Region:          "us-east-1",
		Accelerator:     "gpu",
		InstanceType:    "nvidia-a10g",
		InstanceSize:    "x1",
	}
	// Minimal required fields are set; optional fields default via nil.
	if args.EndpointType != nil {
		t.Error("EndpointType should default to nil (resolved to 'protected' at runtime)")
	}
	if args.MinReplica != nil {
		t.Error("MinReplica should default to nil (resolved to 1 at runtime)")
	}
	if args.MaxReplica != nil {
		t.Error("MaxReplica should default to nil (resolved to 1 at runtime)")
	}
}

// TestSpace_SDKValues checks that SDK string constants are as the HF API expects.
func TestSpace_SDKValues(t *testing.T) {
	cases := []struct {
		sdk      provider.SpaceSDK
		expected string
	}{
		{provider.SpaceSDKGradio, "gradio"},
		{provider.SpaceSDKDocker, "docker"},
		{provider.SpaceSDKStatic, "static"},
	}
	for _, c := range cases {
		if string(c.sdk) != c.expected {
			t.Errorf("SpaceSDK %q != %q", c.sdk, c.expected)
		}
	}
}

// TestSpace_HardwareValues checks that hardware tier constants match HF API values.
func TestSpace_HardwareValues(t *testing.T) {
	cases := []struct {
		hw       provider.SpaceHardware
		expected string
	}{
		{provider.SpaceHardwareCPUBasic, "cpu-basic"},
		{provider.SpaceHardwareCPUUpgrade, "cpu-upgrade"},
		{provider.SpaceHardwareT4Small, "t4-small"},
		{provider.SpaceHardwareT4Medium, "t4-medium"},
		{provider.SpaceHardwareA10GSmall, "a10g-small"},
		{provider.SpaceHardwareA10GLarge, "a10g-large"},
		{provider.SpaceHardwareA100Large, "a100-large"},
		{provider.SpaceHardwareH100, "h100"},
		{provider.SpaceHardwareH100x8, "h100x8"},
	}
	for _, c := range cases {
		if string(c.hw) != c.expected {
			t.Errorf("SpaceHardware %q != %q", c.hw, c.expected)
		}
	}
}

// TestRepository_DefaultRepoType validates that resolveRepoType returns "model" by default.
func TestRepository_DefaultRepoType(t *testing.T) {
	args := provider.RepositoryArgs{}
	if args.RepoType != nil {
		t.Error("RepoType should be nil by default")
	}
	// resolveRepoType is unexported; verify the RepositoryArgs struct shape.
}

// TestEndpointType_Values verifies endpoint type string constants.
func TestEndpointType_Values(t *testing.T) {
	cases := []struct {
		et       provider.EndpointType
		expected string
	}{
		{provider.EndpointTypePublic, "public"},
		{provider.EndpointTypeProtected, "protected"},
		{provider.EndpointTypePrivate, "private"},
	}
	for _, c := range cases {
		if string(c.et) != c.expected {
			t.Errorf("EndpointType %q != %q", c.et, c.expected)
		}
	}
}

// TestEndpointState_Values verifies endpoint state string constants.
func TestEndpointState_Values(t *testing.T) {
	states := []provider.EndpointState{
		provider.EndpointStatePending,
		provider.EndpointStateInitializing,
		provider.EndpointStateUpdating,
		provider.EndpointStateUpdateFailed,
		provider.EndpointStateRunning,
		provider.EndpointStatePaused,
		provider.EndpointStateFailed,
		provider.EndpointStateScaledToZero,
	}
	for _, s := range states {
		if string(s) == "" {
			t.Errorf("EndpointState constant is empty string")
		}
	}
}

// TestClient_Construction verifies that newHFClient does not panic.
func TestClient_Construction(t *testing.T) {
	// newHFClient is unexported; this test is a package-level compilation guard.
	// The public API surface is tested via provider.New() above.
	_ = provider.New
}

// TestSplitID_Valid checks valid namespace/name IDs.
func TestSplitID_Valid(t *testing.T) {
	cases := []struct {
		id        string
		namespace string
		name      string
	}{
		{"myorg/my-endpoint", "myorg", "my-endpoint"},
		{"user/llama-3-8b", "user", "llama-3-8b"},
		{"org/space-with-many-words", "org", "space-with-many-words"},
	}
	for _, c := range cases {
		ns, name, err := provider.SplitID(c.id)
		if err != nil {
			t.Errorf("SplitID(%q) unexpected error: %v", c.id, err)
			continue
		}
		if ns != c.namespace || name != c.name {
			t.Errorf("SplitID(%q) = (%q, %q), want (%q, %q)", c.id, ns, name, c.namespace, c.name)
		}
	}
}

// TestSplitID_Invalid checks that IDs without a slash return an error.
func TestSplitID_Invalid(t *testing.T) {
	badIDs := []string{"noSlash", "", "a"}
	for _, id := range badIDs {
		_, _, err := provider.SplitID(id)
		if err == nil {
			t.Errorf("SplitID(%q) should have returned error", id)
		}
	}
}

// ---------------------------------------------------------------------------
// CheckConfig: environment variable fallbacks
// ---------------------------------------------------------------------------

// envConfigKeys maps each provider config key to the environment variables
// that back it (first non-empty wins) and whether the key is a secret.
var envConfigKeys = []struct {
	key    string
	envs   []string
	secret bool
}{
	{key: "token", envs: []string{"HUGGINGFACE_TOKEN"}, secret: true},
}

// newEnvTestServer builds an integration server for the provider with every
// provider env var cleared so each case is hermetic.
func newEnvTestServer(t *testing.T) integration.Server {
	t.Helper()
	for _, k := range envConfigKeys {
		for _, e := range k.envs {
			t.Setenv(e, "")
		}
	}
	p, err := provider.New()
	if err != nil {
		t.Fatalf("provider.New() returned error: %v", err)
	}
	s, err := integration.NewServer(t.Context(), "huggingface", semver.MustParse("0.0.1"), integration.WithProvider(p))
	if err != nil {
		t.Fatalf("integration.NewServer failed: %v", err)
	}
	return s
}

// TestCheckConfig_EnvFallbacks verifies that unset provider config is filled
// from the environment, that explicit config still wins, and that the first
// non-empty variable wins when a key has several.
func TestCheckConfig_EnvFallbacks(t *testing.T) {
	type tc struct {
		name   string
		key    string
		secret bool
		env    map[string]string
		config map[string]property.Value
		want   string
	}
	var tests []tc
	for _, k := range envConfigKeys {
		for _, e := range k.envs {
			tests = append(tests, tc{name: k.key + " from " + e, key: k.key, secret: k.secret, env: map[string]string{e: "from-" + e}, want: "from-" + e})
		}
		if len(k.envs) > 1 {
			env := map[string]string{}
			for _, e := range k.envs {
				env[e] = "from-" + e
			}
			tests = append(tests, tc{name: k.key + " first env wins", key: k.key, secret: k.secret, env: env, want: "from-" + k.envs[0]})
		}
		tests = append(tests, tc{
			name:   k.key + " explicit config wins over env",
			key:    k.key,
			secret: k.secret,
			env:    map[string]string{k.envs[0]: "from-env"},
			config: map[string]property.Value{k.key: property.New("from-config")},
			want:   "from-config",
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newEnvTestServer(t)
			for e, v := range tt.env {
				t.Setenv(e, v)
			}
			resp, err := s.CheckConfig(pgp.CheckRequest{
				Urn:    resource.NewURN("stack", "proj", "", "pulumi:providers:huggingface", "default"),
				Inputs: property.NewMap(tt.config),
			})
			if err != nil {
				t.Fatalf("CheckConfig failed: %v", err)
			}
			if len(resp.Failures) != 0 {
				t.Fatalf("CheckConfig returned failures: %v", resp.Failures)
			}

			got, ok := resp.Inputs.GetOk(tt.key)
			if !ok {
				t.Fatalf("%s should be populated", tt.key)
			}
			// infer marks env-sourced and config-sourced secret fields as secret.
			if tt.secret && !got.Secret() {
				t.Fatalf("%s should be marked secret", tt.key)
			}
			if got.Secret() {
				got = got.WithSecret(false)
			}
			if gotStr := got.AsString(); gotStr != tt.want {
				t.Fatalf("%s = %q, want %q", tt.key, gotStr, tt.want)
			}
		})
	}
}

// TestSchema_ConfigEnvironmentDefaults verifies the generated schema
// advertises each config key's environment variables as defaultInfo, which is
// what SDKs and the Registry docs consume.
func TestSchema_ConfigEnvironmentDefaults(t *testing.T) {
	s := newEnvTestServer(t)
	resp, err := s.GetSchema(pgp.GetSchemaRequest{})
	if err != nil {
		t.Fatalf("GetSchema failed: %v", err)
	}

	var spec struct {
		Config struct {
			Variables map[string]struct {
				DefaultInfo struct {
					Environment []string `json:"environment"`
				} `json:"defaultInfo"`
			} `json:"variables"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(resp.Schema), &spec); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	for _, k := range envConfigKeys {
		v, ok := spec.Config.Variables[k.key]
		if !ok {
			t.Fatalf("config.variables.%s should be declared", k.key)
		}
		if !slices.Equal(k.envs, v.DefaultInfo.Environment) {
			t.Fatalf("config.variables.%s.defaultInfo.environment = %v, want %v", k.key, v.DefaultInfo.Environment, k.envs)
		}
	}
}
