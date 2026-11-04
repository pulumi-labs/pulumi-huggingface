---
title: Hugging Face Installation & Configuration
meta_desc: How to install the Pulumi Hugging Face provider and its language SDKs, and how to configure credentials.
layout: package
---

## Installation

The Hugging Face provider is available as a package in all Pulumi-supported languages:

* JavaScript/TypeScript: [`@pulumi-labs/huggingface`](https://www.npmjs.com/package/@pulumi-labs/huggingface)
* Python: [`pulumi-labs-huggingface`](https://pypi.org/project/pulumi-labs-huggingface/)
* Go: [`github.com/pulumi-labs/pulumi-huggingface/sdk/go/huggingface`](https://github.com/pulumi-labs/pulumi-huggingface)
* .NET: [`Pulumi.Labs.Huggingface`](https://www.nuget.org/packages/Pulumi.Labs.Huggingface)
* Java: [`com.pulumi.labs:huggingface`](https://central.sonatype.com/artifact/com.pulumi.labs/huggingface)
In normal usage, installing one of the language SDKs above is enough. The first `pulumi up` automatically downloads the matching provider binary from the URL embedded in the SDK, so no separate step is required.

### Provider Binary (optional)

If you need to install the provider binary explicitly (for example, in an air-gapped environment or to prewarm a CI cache), use the `pulumi plugin` command:

```bash
pulumi plugin install resource huggingface <version> --server github://api.github.com/pulumi-labs/pulumi-huggingface
```

Replace `<version>` with your desired release (see the [releases page](https://github.com/pulumi-labs/pulumi-huggingface/releases)).

## Configuration

Create a Hugging Face API token with **write** access from your [Hugging Face settings](https://huggingface.co/settings/tokens). The provider needs no explicit `Provider` resource and no stack config when the token is available as an environment variable:

```bash
export HUGGINGFACE_TOKEN=hf_xxxxxxxxxxxxxxxx
```

You can also set it as provider configuration:

```bash
pulumi config set --secret huggingface:token hf_xxxxxxxxxxxxxxxx
```

Each resource also accepts a `token` input, so a single stack can provision resources under different Hugging Face accounts or organizations. The lookup order is the resource's `token`, then `huggingface:token`, then the `HUGGINGFACE_TOKEN` environment variable.
