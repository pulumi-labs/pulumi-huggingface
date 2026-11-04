# Changelog

## [Unreleased]

### Changed

- `go.mod`/`go.sum` moved from `provider/` to the provider's top level (module path is now `github.com/pulumi/autoregistry/lib/providers/huggingface`); the Makefile builds from there. No change to the provider binary, schema or SDKs.
- Examples and templates are mirror-ready: all 6 examples and 12 templates use the default provider (no explicit `Provider` resource, no per-resource `token` argument), so credentials come from `pulumi config set huggingface:token --secret` or `HUGGINGFACE_TOKEN`. The `huggingface-csharp` example and the `huggingface-cloud-csharp` template no longer hit the ambiguous `Config` compile error.

### Fixed

- `InferenceEndpoint`: BREAKING: the endpoint type `protected` was removed by the Inference Endpoints API. `endpointType` now defaults to `authenticated` (it used to default to `protected` on every create and update) and `protected` is rejected at preview with a message pointing to `authenticated`. Examples, templates and the README use `authenticated`; change any program or stack config that sets `protected`. Existing endpoints that were created as `protected` and show a type diff will be updated to `authenticated`.
- `InferenceEndpoint`: create now sends the required `model.image` (`{"huggingface": {}}` for the stock image, `{"custom": {...}}` with `customImageUrl`), puts the container health route at `healthRoute` and the environment variables at `model.env` (they were silently dropped before), and sends `accountId` as `privateService.accountId`. New optional input `privateLinkRegion` (defaults to `region`) completes the PrivateLink configuration.
- `InferenceEndpoint`: update now sends everything the API can change (`modelRepository`, `modelRevision`, `modelFramework`, `modelTask`, custom image URL/port/health route, env, `vendor`, `region`, compute and scaling, `endpointType`); before, only type and compute were sent while state recorded the new values. `accountId` and `privateLinkRegion` replace the endpoint on change.
- `InferenceEndpoint`: create and update wait until the endpoint leaves `pending`/`initializing`/`updating`, so `url` and `status` are populated on return (templates that feed `endpoint.url` into a Space variable no longer get an empty value). A failed endpoint fails the operation but is recorded in state.
- `InferenceEndpoint`: refresh reads model, image, env, compute, scaling, type, provider and private link back from the API and no longer blanks `namespace`; out-of-band changes show in the preview and are reverted by `pulumi up`. The resource can be imported by `namespace/name` with all inputs populated.
- `Space`: variables and secrets are written one key per request (`POST` one `{key,value,description}`; `DELETE` for removed keys), not as a JSON array, and clearing the lists now deletes the keys. Update also applies `private` (settings), `hardware` and `sleepTimeSeconds` (`sleeptime`).
- BREAKING: `Space` `sdk`, `namespace` and `Repository` `repoType`, `namespace` (and `InferenceEndpoint` `namespace`) now replace the resource when changed instead of planning an update that did nothing. Replacements delete before creating because the Hugging Face name is the Pulumi resource name.
- `Space`: refresh reads hardware from `runtime.hardware` (it was read from a top-level field that does not exist), plus `sdk`, `private`, sleep time, variables and secret keys, so drift is detected. Space `sdk` accepts `streamlit`. The invalid hardware names `h100` and `h100x8` are rejected at preview; the Hub's flavors (`l4x1`, `l40sx1`, `a100x4`, `a100x8`, ...) are documented.
- `Repository`: visibility (`private`) changes are now applied through the repo settings API; update used to be a no-op. Refresh updates inputs so `private` drift is reverted. BREAKING (import ID): dataset repositories now use the resource ID `datasets/namespace/name` (models stay `namespace/name`) so they can be imported by ID alone; existing dataset state with the old ID keeps working.
- `InferenceEndpoint`, `Space`, `Repository`: deleting a resource that was already deleted outside Pulumi is a no-op instead of an error; refresh with no token configured now fails with a clear error instead of silently returning stale state.
- Docs: `scaleToZeroTimeout` API default is 15 minutes (README said 60); the `sleepTimeSeconds` default claim was removed (unset means unmanaged).
- Tests: added an in-memory Hugging Face fake and `integration.Server` lifecycle tests (create, no-op diff, update, drift, import, deleted-outside) for all three resources, replacing the two assertion-free placeholder tests.
- `huggingface` provider: the `huggingface:token` config key now falls back to the `HUGGINGFACE_TOKEN` environment variable when unset (the previous struct tag was ignored by the framework). Explicit config still wins.

### Added

- `templates/inference-endpoint-huggingface-cloud-<lang>` starters (all 6 languages) — scenario template that provisions a dedicated `InferenceEndpoint` plus a Gradio `Space` wired to call it, the production model-serving pattern for HF.
- `templates/` directory with six `pulumi new`-compatible starters (`huggingface-cloud-typescript`, `-python`, `-go`, `-csharp`, `-java`, `-yaml`). Each starter provisions a free-tier Hugging Face model `Repository` plus a Gradio `Space` on `cpu-basic` hardware — a zero-cost onramp to the canonical HF Hub workflow.
- `InferenceEndpoint` resource — provision dedicated GPU/CPU inference endpoints on Hugging Face with full CRUD lifecycle, autoscaling, scale-to-zero, and custom container support (TGI, TEI, vLLM)
- `Space` resource — create and manage Hugging Face Spaces (Gradio, Docker, static) with hardware tier selection, secrets, and environment variables
- `Repository` resource — manage Hugging Face Hub model and dataset repository containers with visibility control
- Provider-level `token` configuration with `HUGGINGFACE_TOKEN` environment variable fallback

