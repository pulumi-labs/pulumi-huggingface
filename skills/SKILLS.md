# Hugging Face Provider Skills

## Component Overview

| Property | Value |
|---|---|
| Type token prefix | `huggingface:index:` |
| Resources | `InferenceEndpoint`, `Space`, `Repository` |
| Upstream API | Hugging Face Hub API + Inference Endpoints API (no TF provider — native implementation) |
| pulumi-go-provider | `v1.3.1` |
| Resource count | 3 |

## Architecture

This is a **native provider** (not a TF bridge). It uses `pulumi-go-provider` with the `infer` package and talks directly to two HF API planes:

- **Inference Endpoints API**: `api.endpoints.huggingface.cloud/v2`
- **Hub API**: `huggingface.co/api`

All HTTP calls go through `pkg/provider/client.go`. No external SDK dependencies — the client is self-contained.

## Development Workflow

| Step | Command | When to run |
|---|---|---|
| 1 | `make tidy` | Fresh checkout, after adding new dependencies |
| 2 | `make build` | After code changes |
| 3 | `make install` | To test locally with Pulumi CLI |
| 4 | `make gen-sdk` | After schema changes (creates `sdk/`) |
| 5 | `make test` | Always before committing |
| 6 | `make fmt` | Always before committing |

**Go toolchain**: The Makefile uses `~/go-toolchain-1.25/go/bin/go` (Go 1.25.8). This is mandatory — do not substitute system `go`.

**Build verification sequence** (always do in this order):
```bash
make tidy
make build
make test
make fmt
```

## Repository Layout

```
lib/providers/huggingface/
├── Makefile                        # Build targets (tidy, build, install, gen-sdk, test, fmt)
├── version.txt                     # Current version (e.g. 0.1.0)
├── CHANGELOG.md                    # Required; use [Unreleased] header
├── README.md                       # Full resource documentation
├── .gitignore                      # Excludes bin/, sdk/, provider/vendor/
├── examples/
│   ├── huggingface-ts/             # TypeScript example (index.ts + package.json + tsconfig.json)
│   ├── huggingface-python/         # Python example (__main__.py + requirements.txt)
│   ├── huggingface-go/             # Go example (main.go + go.mod)
│   ├── huggingface-csharp/         # C# example (Program.cs + .csproj)
│   ├── huggingface-java/           # Java example (App.java + pom.xml)
│   └── huggingface-yaml/           # YAML example (Pulumi.yaml with inline resources)
├── provider/                       # Go module (single module, no shim — native provider)
│   ├── go.mod                      # module github.com/.../huggingface/provider
│   ├── go.sum
│   ├── cmd/
│   │   └── pulumi-resource-huggingface/
│   │       └── main.go             # Binary entrypoint: calls pgp.RunProvider
│   └── pkg/
│       ├── provider/
│       │   ├── provider.go         # infer.NewProviderBuilder() wiring
│       │   ├── client.go           # HF HTTP client (no external SDK)
│       │   ├── inference_endpoint.go  # InferenceEndpoint CRUD + helpers
│       │   ├── space.go            # Space CRUD
│       │   ├── repository.go       # Repository CRUD
│       │   └── provider_test.go    # Unit tests
│       └── version/
│           └── version.go          # Version var injected by -ldflags
└── skills/
    └── SKILLS.md                   # This file
```

**Source vs generated:**
- Source: everything in `provider/` and `examples/` except `go.sum` (which go tool manages)
- Generated (do NOT commit): `bin/`, `sdk/`

## Common Mistakes

| Mistake | Consequence | Correct approach |
|---|---|---|
| Using `pgp.RunProvider` with old flat CRUD signature | Build fails with interface mismatch | Use `infer.CreateRequest[I]`, `infer.ReadRequest[I,O]`, etc. — all CRUD methods take request structs and return response structs |
| Returning just `error` from `Delete` | Build fails | `Delete` returns `(infer.DeleteResponse, error)` not just `error` |
| Calling `infer.ProviderConfig[T]()` | Build fails — function does not exist | Use `infer.Config(T{})` |
| Calling `infer.Resource[R, I, O]()` with no argument | Build fails — Resource takes an instance | Use `infer.Resource[R, I, O](&R{})` where R is the concrete type |
| Calling `infer.ComponentF` for custom resources | Registers as component, not resource | `ComponentF` is for Component Resources; use `infer.Resource` for CRUD resources |
| Forgetting to export `SplitID` for tests | Tests in `_test` package can't call it | Export as `SplitID` in the source file; keep internal `splitID` alias |
| Passing `namespace` in `DeleteRepo` body with key `"organization"` | Works — but only when namespace is an org, not a personal account | Always check: if namespace == token owner username, omit `organization` from delete payload |
| Setting `customImageUrl` without `modelFramework: "pytorch"` or `"custom"` | API rejects request | When using TGI/TEI/vLLM, set `modelFramework: "pytorch"` or `"custom"` |
| Hardcoding instance types | Works but is fragile across vendors | Available instance types vary by vendor/region — surface them via `GET /v2/provider` if needed |

## API Quirks

### Inference Endpoints API

- **Namespace resolution**: If `namespace` is not provided, the provider calls `GET /api/whoami-v2` to get the authenticated user's name. This requires a valid token on every Create.
- **Endpoint ID format**: `namespace/name` — always two segments. Never three.
- **Status on Create**: Endpoints are created in `pending` → `initializing` → `running` state. The `url` field is only populated once the state is `running`. Pulumi state will show `url: ""` while initializing — this is normal.
- **Scale-to-zero**: Requires `minReplica: 0` AND a `scaleToZeroTimeout` value. Without both, scale-to-zero does not activate.
- **Custom image port**: If omitted, HF defaults to port 8080. Many TGI images expect 8080.
- **Update semantics**: The `PUT /endpoint/{namespace}/{name}` endpoint only accepts `compute`, `model`, and `type` changes. Model repository changes trigger redeployment. Name changes are NOT supported (force replace).

### Hub API

- **Repo creation response**: The `id` field in the create response may be empty for some repo types. Always fall back to constructing `namespace/name` manually when `id == ""`.
- **Secrets are write-only**: The Hub API does NOT return secret values on Read. State drift on secrets cannot be detected — this is intentional. To rotate secrets, update them and re-run `pulumi up`.
- **Space hardware updates**: Use `POST /api/spaces/{namespace}/{name}/hardware` with `{"flavor": "..."}`. This is a separate endpoint from repo creation.
- **Repo deletion**: Uses `DELETE /api/repos/delete` with a JSON body (not URL params) containing `name`, `type`, and optionally `organization`. This is unusual — most REST APIs use URL params for DELETE.

## HF API Authentication

All requests use `Authorization: Bearer <token>` header. Token scopes:
- `read` — sufficient for Get/List operations only
- `write` — required for Create/Update/Delete on all resource types
- Fine-grained tokens are supported but must include explicit write access for each namespace

Token priority order (per resource):
1. `token` input on the resource (if set)
2. `token` in provider configuration
3. `HUGGINGFACE_TOKEN` environment variable (via `env` struct tag on ProviderConfig)

## Test Coverage

Tests in `pkg/provider/provider_test.go` cover:
- `TestProvider_DoesNotPanic` — schema construction sanity check
- `TestProviderConfig_TokenIsSecret` — compile-time secret annotation guard
- `TestInferenceEndpoint_ArgDefaults` — nil optional inputs
- `TestSpace_SDKValues` — string constant correctness
- `TestSpace_HardwareValues` — all 9 hardware tier constants
- `TestRepository_DefaultRepoType` — nil repoType default
- `TestEndpointType_Values` — 3 endpoint type constants
- `TestEndpointState_Values` — 8 lifecycle state constants
- `TestClient_Construction` — package compilation guard
- `TestSplitID_Valid` — 3 valid namespace/name patterns
- `TestSplitID_Invalid` — 3 invalid ID patterns (no slash, empty, single char)
