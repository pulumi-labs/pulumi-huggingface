# Changelog

## [Unreleased]

### Added

- `templates/` directory with six `pulumi new`-compatible starters (`huggingface-cloud-typescript`, `-python`, `-go`, `-csharp`, `-java`, `-yaml`). Each starter provisions a free-tier Hugging Face model `Repository` plus a Gradio `Space` on `cpu-basic` hardware — a zero-cost onramp to the canonical HF Hub workflow.
- `InferenceEndpoint` resource — provision dedicated GPU/CPU inference endpoints on Hugging Face with full CRUD lifecycle, autoscaling, scale-to-zero, and custom container support (TGI, TEI, vLLM)
- `Space` resource — create and manage Hugging Face Spaces (Gradio, Docker, static) with hardware tier selection, secrets, and environment variables
- `Repository` resource — manage Hugging Face Hub model and dataset repository containers with visibility control
- Provider-level `token` configuration with `HUGGINGFACE_TOKEN` environment variable fallback

