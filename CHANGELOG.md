# Changelog

## [Unreleased]

### Changed

- `go.mod`/`go.sum` moved from `provider/` to the provider's top level (module path is now `github.com/pulumi/autoregistry/lib/providers/huggingface`); the Makefile builds from there. No change to the provider binary, schema or SDKs.
- Examples and templates are mirror-ready: all 6 examples and 12 templates use the default provider (no explicit `Provider` resource, no per-resource `token` argument), so credentials come from `pulumi config set huggingface:token --secret` or `HUGGINGFACE_TOKEN`. The `huggingface-csharp` example and the `huggingface-cloud-csharp` template no longer hit the ambiguous `Config` compile error.

### Fixed

- `huggingface` provider: the `huggingface:token` config key now falls back to the `HUGGINGFACE_TOKEN` environment variable when unset (the previous struct tag was ignored by the framework). Explicit config still wins.

### Added

- `templates/inference-endpoint-huggingface-cloud-<lang>` starters (all 6 languages) — scenario template that provisions a dedicated `InferenceEndpoint` plus a Gradio `Space` wired to call it, the production model-serving pattern for HF.
- `templates/` directory with six `pulumi new`-compatible starters (`huggingface-cloud-typescript`, `-python`, `-go`, `-csharp`, `-java`, `-yaml`). Each starter provisions a free-tier Hugging Face model `Repository` plus a Gradio `Space` on `cpu-basic` hardware — a zero-cost onramp to the canonical HF Hub workflow.
- `InferenceEndpoint` resource — provision dedicated GPU/CPU inference endpoints on Hugging Face with full CRUD lifecycle, autoscaling, scale-to-zero, and custom container support (TGI, TEI, vLLM)
- `Space` resource — create and manage Hugging Face Spaces (Gradio, Docker, static) with hardware tier selection, secrets, and environment variables
- `Repository` resource — manage Hugging Face Hub model and dataset repository containers with visibility control
- Provider-level `token` configuration with `HUGGINGFACE_TOKEN` environment variable fallback

