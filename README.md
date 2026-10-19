# Hugging Face Pulumi Provider

Manage [Hugging Face](https://huggingface.co) resources as infrastructure with Pulumi — across TypeScript, Python, Go, C#, Java, and YAML.

This provider covers the resources that AI/ML engineering teams care about most: inference endpoints, Spaces, and Hub repositories. Declare them in code, version them in Git, and deploy them with the same workflow as the rest of your cloud infrastructure.

## Installation

Install the provider package for your language:

```bash
# TypeScript / JavaScript
npm install @pulumi/huggingface

# Python
pip install pulumi-huggingface

# Go
go get github.com/pulumi/pulumi-huggingface/sdk/go/huggingface

# .NET
dotnet add package Pulumi.Huggingface

# Java (Maven)
# Add com.pulumi:huggingface to your pom.xml

# YAML — no package install required
```

## Authentication

Set your Hugging Face API token in provider configuration or as an environment variable:

```bash
# Environment variable (recommended for CI/CD)
export HUGGINGFACE_TOKEN=hf_...

# Or in Pulumi config (stored as a secret)
pulumi config set --secret huggingface:token hf_...
```

Examples and templates use the default provider and never pass the token per resource, so the same `huggingface:token` / `HUGGINGFACE_TOKEN` setting works everywhere. The inference-endpoint templates additionally read an `hfToken` secret only to store it as the Space `HF_TOKEN` secret.

Tokens must have **write** access to create endpoints, Spaces, and repositories. Generate one at [huggingface.co/settings/tokens](https://huggingface.co/settings/tokens).

## Resources

### `InferenceEndpoint`

Deploy a dedicated compute instance that serves a Hugging Face model for production inference. Supports public, protected, and private access types across AWS, Azure, and GCP.

**Key features:**
- GPU and CPU hardware (NVIDIA A10G, H100, Intel Ice Lake, AWS Inferentia 2)
- Autoscaling with `minReplica` / `maxReplica`
- Scale-to-zero when `minReplica: 0` (saves cost during idle periods)
- Custom container images for TGI, TEI, vLLM, SGLang
- Private VPC endpoints via AWS PrivateLink

#### Inputs

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `modelRepository` | string | ✓ | — | Model repo ID (e.g. `"meta-llama/Llama-3-8B-Instruct"`) |
| `modelFramework` | string | ✓ | — | Serving framework: `"pytorch"`, `"custom"` |
| `modelTask` | string | ✓ | — | Task type: `"text-generation"`, `"feature-extraction"`, etc. |
| `vendor` | string | ✓ | — | Cloud vendor: `"aws"`, `"azure"`, `"gcp"` |
| `region` | string | ✓ | — | Cloud region (e.g. `"us-east-1"`) |
| `accelerator` | string | ✓ | — | Hardware: `"cpu"`, `"gpu"`, `"inf2"` |
| `instanceType` | string | ✓ | — | Instance family (e.g. `"nvidia-a10g"`, `"intel-icl"`) |
| `instanceSize` | string | ✓ | — | Instance size: `"x1"`, `"x2"`, `"x4"`, `"x8"` |
| `token` | string | — | provider token | Per-resource API token override |
| `namespace` | string | — | token owner | HF user or org namespace |
| `modelRevision` | string | — | latest | Git SHA or branch to pin the model |
| `customImageUrl` | string | — | — | Custom container image URL (e.g. TGI image) |
| `customImageEnvVars` | map[string]string | — | — | Environment variables for custom container |
| `customImagePort` | int | — | 8080 | Port override for custom container |
| `customImageHealthRoute` | string | — | — | Health check path (e.g. `"/health"`) |
| `minReplica` | int | — | 1 | Minimum replicas (0 = scale-to-zero enabled) |
| `maxReplica` | int | — | 1 | Maximum replicas for autoscaling |
| `scaleToZeroTimeout` | int | — | 60 | Idle minutes before scaling to zero |
| `endpointType` | string | — | `"protected"` | Access type: `"public"`, `"protected"`, `"private"` |
| `accountId` | string | — | — | AWS account ID for PrivateLink (private endpoints) |

#### Outputs

| Output | Type | Description |
|---|---|---|
| `url` | string | HTTPS inference URL (set when endpoint is running) |
| `status` | string | Lifecycle state: `pending`, `initializing`, `running`, `paused`, `scaledToZero`, `failed` |

#### Example: LLM with scale-to-zero

```typescript
import * as pulumi from "@pulumi/pulumi";
import * as huggingface from "@pulumi/huggingface";

const endpoint = new huggingface.InferenceEndpoint("llama-3-8b", {
    modelRepository: "meta-llama/Meta-Llama-3-8B-Instruct",
    modelFramework: "pytorch",
    modelTask: "text-generation",
    vendor: "aws",
    region: "us-east-1",
    accelerator: "gpu",
    instanceType: "nvidia-a10g",
    instanceSize: "x1",
    minReplica: 0,
    maxReplica: 2,
    scaleToZeroTimeout: 60,
    endpointType: "protected",
});

export const endpointUrl = endpoint.url;
```

#### Example: TGI with custom container

```python
import pulumi
import pulumi_huggingface as huggingface

endpoint = huggingface.InferenceEndpoint(
    "mistral-7b-tgi",
    model_repository="mistralai/Mistral-7B-Instruct-v0.3",
    model_framework="pytorch",
    model_task="text-generation",
    custom_image_url="ghcr.io/huggingface/text-generation-inference:2.4.0",
    custom_image_env_vars={
        "MAX_BATCH_TOTAL_TOKENS": "32768",
        "MAX_INPUT_LENGTH": "4096",
    },
    vendor="aws",
    region="us-east-1",
    accelerator="gpu",
    instance_type="nvidia-a10g",
    instance_size="x1",
    min_replica=1,
    max_replica=4,
    endpoint_type="protected",
)
```

---

### `Space`

Create and manage a [Hugging Face Space](https://huggingface.co/docs/hub/spaces) — an interactive demo or application backed by Gradio, Docker, or static HTML.

#### Inputs

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `sdk` | string | ✓ | — | Frontend framework: `"gradio"`, `"docker"`, `"static"` |
| `token` | string | — | provider token | Per-resource API token override |
| `namespace` | string | — | token owner | HF user or org namespace |
| `hardware` | string | — | `"cpu-basic"` | Compute tier (see hardware options below) |
| `private` | bool | — | `false` | Whether the Space is private |
| `sleepTimeSeconds` | int | — | 3600 | Idle seconds before hibernation; -1 to disable |
| `secrets` | list | — | — | Secret env vars (values are write-only) |
| `variables` | list | — | — | Non-secret env vars |

**Hardware options:** `cpu-basic`, `cpu-upgrade`, `t4-small`, `t4-medium`, `a10g-small`, `a10g-large`, `a100-large`, `h100`, `h100x8`

#### Outputs

| Output | Type | Description |
|---|---|---|
| `spaceId` | string | Canonical ID: `namespace/name` |
| `url` | string | Public URL of the Space |

---

### `Repository`

Create and manage a Hugging Face Hub repository — a model repo or dataset repo. Manages the repository container and visibility settings; file management is handled separately (e.g. via the `huggingface_hub` Python library or Git).

#### Inputs

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `token` | string | — | provider token | Per-resource API token override |
| `namespace` | string | — | token owner | HF user or org namespace |
| `repoType` | string | — | `"model"` | Repository type: `"model"` or `"dataset"` |
| `private` | bool | — | `false` | Whether the repository is private |

#### Outputs

| Output | Type | Description |
|---|---|---|
| `repoId` | string | Canonical ID: `namespace/name` |
| `url` | string | Web URL of the repository |

---

## Common Patterns

### AI stack: endpoint + Space demo

```typescript
const endpoint = new huggingface.InferenceEndpoint("llm", {
    modelRepository: "meta-llama/Meta-Llama-3-8B-Instruct",
    modelFramework: "pytorch",
    modelTask: "text-generation",
    vendor: "aws",
    region: "us-east-1",
    accelerator: "gpu",
    instanceType: "nvidia-a10g",
    instanceSize: "x1",
    minReplica: 0,
    maxReplica: 2,
    scaleToZeroTimeout: 60,
});

const config = new pulumi.Config();
const demo = new huggingface.Space("llm-demo", {
    sdk: "gradio",
    hardware: "cpu-basic",
    variables: [{ key: "ENDPOINT_URL", value: endpoint.url }],
    secrets: [{ key: "HF_TOKEN", value: config.requireSecret("hfToken") }],
});
```

### Private model repository with dataset

```python
model_repo = huggingface.Repository(
    "my-fine-tuned-llm",
    namespace="my-org",
    repo_type="model",
    private=True,
)

training_data = huggingface.Repository(
    "training-dataset",
    namespace="my-org",
    repo_type="dataset",
    private=True,
)
```

## Instance Types Quick Reference

| Vendor | Accelerator | Instance Type | Sizes |
|---|---|---|---|
| AWS | GPU | `nvidia-a10g` | x1, x2, x4 |
| AWS | GPU | `nvidia-t4-g4dn` | x1, x4 |
| AWS | GPU | `nvidia-a100` | x1, x4, x8 |
| AWS | GPU | `nvidia-h100` | x1, x4, x8 |
| AWS | CPU | `intel-icl` | x2, x4, x8 |
| AWS | Neuron | `inf2` | x1, x12 |
| Azure | GPU | `nvidia-a10-g` | x1, x2, x4 |
| GCP | GPU | `nvidia-l4` | x1, x4 |

> Instance types are discovered dynamically by the HF API. Use `GET /v2/provider` to list all available hardware for your vendor/region.

## Development

```bash
# Install Go 1.25.8+ and the Pulumi CLI, then:
make tidy     # resolve modules
make build    # compile provider binary
make install  # install binary to ~/.pulumi/plugins/
make gen-sdk  # generate language SDKs
make test     # run unit tests
make fmt      # format Go source
```
