# Hugging Face Pulumi Provider

> [!NOTE]
> **Experimental.** This provider is experimental, versions are `0.x`, and its API may change without notice.

Manage [Hugging Face](https://huggingface.co) resources as infrastructure with Pulumi — across TypeScript, Python, Go, C#, Java, and YAML.

This provider covers the resources that AI/ML engineering teams care about most: inference endpoints, Spaces, and Hub repositories. Declare them in code, version them in Git, and deploy them with the same workflow as the rest of your cloud infrastructure.

## Installation

Install the provider package for your language:

```bash
# TypeScript / JavaScript
npm install @pulumi-labs/huggingface

# Python
pip install pulumi-labs-huggingface

# Go
go get github.com/pulumi-labs/pulumi-huggingface/sdk/go/huggingface

# .NET
dotnet add package Pulumi.Labs.Huggingface

# Java (Maven)
# Add com.pulumi.labs:huggingface to your pom.xml

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

Deploy a dedicated compute instance that serves a Hugging Face model for production inference. Supports public, authenticated, and private access types across AWS, Azure, and GCP.

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
| `namespace` | string | — | token owner | HF user or org namespace. Changing it replaces the resource |
| `modelRevision` | string | — | latest | Git SHA or branch to pin the model |
| `customImageUrl` | string | — | — | Custom container image URL (e.g. TGI image) |
| `customImageEnvVars` | map[string]string | — | — | Environment variables for custom container |
| `customImagePort` | int | — | 8080 | Port override for custom container |
| `customImageHealthRoute` | string | — | — | Health check path (e.g. `"/health"`) |
| `minReplica` | int | — | 1 | Minimum replicas (0 = scale-to-zero enabled) |
| `maxReplica` | int | — | 1 | Maximum replicas for autoscaling |
| `scaleToZeroTimeout` | int | — | 15 (API default) | Idle minutes before scaling to zero |
| `endpointType` | string | — | `"authenticated"` | Access type: `"public"`, `"authenticated"`, `"private"` |
| `accountId` | string | — | — | AWS account ID for PrivateLink (private endpoints); sent as `privateService.accountId`. Changing it replaces the endpoint |
| `privateLinkRegion` | string | — | `region` | AWS PrivateLink entry-point region (`"us-east-1"` or `"eu-west-1"`). Changing it replaces the endpoint |
| `tags` | list[string] | — | unmanaged | Labels attached to the endpoint. Unmanaged while unset; removing tags that were set clears them |

#### Outputs

| Output | Type | Description |
|---|---|---|
| `url` | string | HTTPS inference URL (create and update wait for the endpoint to leave `pending`/`initializing`/`updating`, so it is set on return) |
| `status` | string | Lifecycle state: `pending`, `initializing`, `updating`, `running`, `paused`, `scaledToZero`, `failed`, `updateFailed` |

Import with `pulumi import huggingface:index:InferenceEndpoint <name> <namespace>/<name>`. Refresh reads the model, image, compute, scaling, type, provider and private-link settings back, so out-of-band changes show up in `pulumi preview` and are reverted by `pulumi up`.

#### Example: LLM with scale-to-zero

```typescript
import * as pulumi from "@pulumi/pulumi";
import * as huggingface from "@pulumi-labs/huggingface";

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
    endpointType: "authenticated",
});

export const endpointUrl = endpoint.url;
```

#### Example: TGI with custom container

```python
import pulumi
import pulumi_labs_huggingface as huggingface

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
    endpoint_type="authenticated",
)
```

---

### `Space`

Create and manage a [Hugging Face Space](https://huggingface.co/docs/hub/spaces) — an interactive demo or application backed by Gradio, Docker, or static HTML.

#### Inputs

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `sdk` | string | ✓ | — | Frontend framework: `"gradio"`, `"streamlit"`, `"docker"`, `"static"`. Changing it replaces the Space |
| `token` | string | — | provider token | Per-resource API token override |
| `namespace` | string | — | token owner | HF user or org namespace. Changing it replaces the resource |
| `hardware` | string | — | `"cpu-basic"` | Compute tier (see hardware options below) |
| `private` | bool | — | `false` | Whether the Space is private |
| `sleepTimeSeconds` | int | — | unmanaged | Idle seconds before the Space sleeps (upgraded hardware only) |
| `secrets` | list | — | — | Secret env vars (values are write-only) |
| `variables` | list | — | — | Non-secret env vars |

**Hardware options:** `cpu-basic`, `cpu-upgrade`, `t4-small`, `t4-medium`, `a10g-small`, `a10g-large`, `a100-large`, `a100x4`, `a100x8`, `l4x1`, `l40sx1`, and the other flavors the Hub lists

#### Outputs

| Output | Type | Description |
|---|---|---|
| `spaceId` | string | Canonical ID: `namespace/name` |
| `url` | string | Public URL of the Space |

Import with `pulumi import huggingface:index:Space <name> <namespace>/<name>`. Variables are imported with their values; secret values are write-only on the Hub, so an import adopts none and the program must supply them. Variables and secrets are synced key by key: keys removed from the program are deleted from the Space.

---

### `Repository`

Create and manage a Hugging Face Hub repository — a model repo or dataset repo. Manages the repository container and visibility settings; file management is handled separately (e.g. via the `huggingface_hub` Python library or Git).

#### Inputs

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `token` | string | — | provider token | Per-resource API token override |
| `namespace` | string | — | token owner | HF user or org namespace. Changing it replaces the resource |
| `repoType` | string | — | `"model"` | Repository type: `"model"` or `"dataset"`. Changing it replaces the repository |
| `private` | bool | — | `false` | Whether the repository is private |
| `gated` | string | — | `"off"` | Gated access: `"off"`, `"auto"` (requests approved automatically) or `"manual"` (a maintainer approves each request) |

#### Outputs

| Output | Type | Description |
|---|---|---|
| `repoId` | string | Canonical ID: `namespace/name` |
| `url` | string | Web URL of the repository |

The Pulumi resource ID is `namespace/name` for a model and `datasets/namespace/name` for a dataset. Import with `pulumi import huggingface:index:Repository <name> <namespace>/<name>` or `... datasets/<namespace>/<name>`. Visibility and gating changes are applied in place with one settings call.

---

### `Webhook`

A Hub webhook that posts events for watched users, organizations, models, datasets, Spaces or buckets to a URL. Webhooks that trigger a Job are not supported yet.

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| `url` | string | ✓ | — | URL that receives the payloads |
| `watched` | list | ✓ | — | Items to watch: objects with `type` (`bucket`, `dataset`, `model`, `org`, `space`, `user`) and `name` |
| `domains` | list[string] | — | both | `repo` and/or `discussion`; the Hub watches both when unset (the Hub's value is the `effectiveDomains` output) |
| `secret` | string (secret) | — | — | Signing secret. Write-only: never returned by the API, kept from state, cannot be imported |
| `disabled` | bool | — | `false` | Pause the webhook (applied with the enable/disable calls) |
| `token` | string | — | provider token | Per-resource API token override |

Import ID: the webhook ID, for example `pulumi import huggingface:index:Webhook hook 64f9a55bb3115b4f513ec026`. Set `secret` again after an import.

### `Collection` and `CollectionItem`

`Collection` is a titled, curated list (`title`, `description`, `private`, `theme`, `namespace`; changing `namespace` replaces it). The Hub chooses a theme when none is set and reports it as `effectiveTheme`. Outputs: `slug`, `url`. Import ID: the slug, `namespace/title-slug-hash`.

`CollectionItem` puts one model, dataset, Space, paper, bucket or collection into a collection: `collectionSlug`, `itemId`, `itemType` (all three replace on change) and an optional `note` (updated in place). Output: `itemObjectId`. Import ID: `<collection slug>/<item object ID>` (the object ID is shown by `getCollection`).

### `RepositoryBranch` and `RepositoryTag`

Git refs of a model, dataset or Space repository. `repoId` (`namespace/name`), optional `repoType` (`model`, `dataset`, `space`) and the ref name replace on change. `startingPoint` (branch) and `revision` / `message` (tag) are create-only and not readable from the API, so they are not detected as drift and are unset after an import. Output: `targetCommit`. Import ID: `<models|datasets|spaces>/namespace/name@<ref>`; without a prefix the repository is a model, for example `pulumi import huggingface:index:RepositoryBranch dev acme/model@dev`.

### `Bucket`

A Hub storage bucket (`namespace`, optional `name` defaulting to the resource name, `private`, `region`, `resourceGroupId`). The documented API has no bucket settings update, so **any change replaces the bucket and deletes its files**. Outputs: `bucketId`, `url`. Import ID: `namespace/name`.

---

## Functions

Lookup functions never create anything. Each takes an optional `token` and fails with a "was not found" message for a missing object.

| Function | Looks up | Notes |
|---|---|---|
| `getWhoami` | the identity behind the token | `name`, `type`, `fullName`, `orgs` |
| `getInferenceEndpoint` | an endpoint by `namespace` (default: token owner) and `name` | status, url, model, compute, scaling, `tags` |
| `listInferenceEndpoints` | the endpoints of a namespace | |
| `getRepository` | a model or dataset by `repoId` and `repoType` | `private`, `gated` |
| `getSpace` | a Space by `spaceId` | variables with values, secret **names** only (secret values are not supported by the Hugging Face API) |
| `getWebhook` | a webhook by `webhookId` | the secret is not part of the result |
| `getCollection` | a collection by `slug` | includes its items |
| `getCollectionItem` | an item by `collectionSlug`, `itemId`, `itemType` | |
| `getRepositoryBranch`, `getRepositoryTag` | a ref by `repoId`, optional `repoType`, and name | `ref`, `targetCommit` |
| `getBucket` | a bucket by `namespace` (default: token owner) and `name` | size, file count |

Example (TypeScript):

```typescript
import * as huggingface from "@pulumi-labs/huggingface";

const me = huggingface.getWhoamiOutput({});
const repo = new huggingface.Repository("model", { gated: "auto" });
const release = new huggingface.RepositoryTag("v1", { repoId: repo.repoId, tag: "v1.0", message: "first release" });
export const owner = me.name;
```

The full list of what the Hugging Face APIs offer and what the provider does with each part is in [`research/coverage/huggingface.md`](../../../research/coverage/huggingface.md).

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
