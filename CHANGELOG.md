# Changelog

## [Unreleased]

### Added

- `InferenceEndpoint` resource — provision dedicated GPU/CPU inference endpoints on Hugging Face with full CRUD lifecycle, autoscaling, scale-to-zero, and custom container support (TGI, TEI, vLLM)
- `Space` resource — create and manage Hugging Face Spaces (Gradio, Docker, static) with hardware tier selection, secrets, and environment variables
- `Repository` resource — manage Hugging Face Hub model and dataset repository containers with visibility control
- Provider-level `token` configuration with `HUGGINGFACE_TOKEN` environment variable fallback
