---
title: Hugging Face Provider
meta_desc: Manage Hugging Face resources (Inference Endpoints, Spaces, Hub repositories, webhooks, collections and buckets) with Pulumi.
layout: package
---

An experimental Pulumi provider for [Hugging Face](https://huggingface.co). It manages Inference Endpoints, Spaces, Hub repositories and related resources as infrastructure.

## Installation

The Hugging Face provider is available as a package in all Pulumi-supported languages:

* JavaScript/TypeScript: [`@pulumi-labs/huggingface`](https://www.npmjs.com/package/@pulumi-labs/huggingface)
* Python: [`pulumi-labs-huggingface`](https://pypi.org/project/pulumi-labs-huggingface/)
* Go: [`github.com/pulumi-labs/pulumi-huggingface/sdk/go/huggingface`](https://github.com/pulumi-labs/pulumi-huggingface)
* .NET: [`Pulumi.Labs.Huggingface`](https://www.nuget.org/packages/Pulumi.Labs.Huggingface)
* Java: [`com.pulumi.labs:huggingface`](https://central.sonatype.com/artifact/com.pulumi.labs/huggingface)
## Example Usage

{{< chooser language "typescript,python,yaml" >}}
{{% choosable language typescript %}}
```typescript
import * as huggingface from "@pulumi-labs/huggingface";

const repo = new huggingface.Repository("demo-model", {
    name: "demo-model",
    repoType: "model",
    private: true,
});

export const repoName = repo.name;
```
{{% /choosable %}}
{{% choosable language python %}}
```python
import pulumi
import pulumi_labs_huggingface as huggingface

repo = huggingface.Repository("demo-model",
    name="demo-model",
    repo_type="model",
    private=True)

pulumi.export("repoName", repo.name)
```
{{% /choosable %}}
{{% choosable language yaml %}}
```yaml
resources:
  demo-model:
    type: huggingface:index:Repository
    properties:
      name: demo-model
      repoType: model
      private: true
outputs:
  repoName: ${demo-model.name}
```
{{% /choosable %}}
{{< /chooser >}}

For runnable programs in every language, see the [`examples/`](https://github.com/pulumi-labs/pulumi-huggingface/tree/main/examples) directory in the provider repository.

## Configuration

The provider works with no explicit `Provider` resource. Set the Hugging Face API token as an environment variable and every resource picks it up. See Installation & Configuration for the other ways to configure credentials.
