# Changelog

## [Unreleased]

## [0.1.3] - 2026-11-08

### Fixed

- `RepositoryTag` and `getRepositoryTag`: `targetCommit` is now always the commit the tag points at. For a tag created with a message (an annotated tag) the Hub refs listing returns the tag object id, which is not a commit in the repository, and it was surfaced as is. The commit is now resolved through the Hub revision endpoint on create, refresh, import and lookup, falling back to the refs value when that call fails. Branches are unchanged. Mirror issue https://github.com/pulumi-labs/pulumi-huggingface/issues/10 (#1675).
- Security: updated the indirect dependency `github.com/klauspost/compress` from v1.18.7 to v1.20.2, which fixes GO-2026-6865 (GHSA-fpmh-frq8-7rj8, a panic on two-block frames). No behaviour changes.

## [0.1.2] - 2026-11-05

### Fixed

- Security: `Read` also marks a secret nested inside an object, an array of objects or a map of objects (`SpaceSecretInput.value` on `Space.secrets`) as a secret, not only top-level properties (#1599). If the provider schema cannot be loaded during a `Read`, the failure is no longer cached for the life of the process: the next `Read` retries (#1599).
- Security: `pulumi import` and `pulumi refresh` now store the properties this provider marks secret (every per-resource `token` and `Webhook.secret`) as secrets. A `Read` response only kept the secretness of the state it was given, and an import has no state, so a value the provider recovered from the API was written to the state as plaintext. The secret properties are read from the provider's own schema, so every resource is covered. State imported before this fix stays plaintext until its next `pulumi refresh`, which re-encrypts it (#1577).
- A preview no longer invents resource IDs. `Create` during `pulumi preview` returned the resource's logical name (or the placeholder `preview`) as its ID, so `id` was a known, wrong value and a program passing it to a lookup or to another resource ran against a name that does not exist and failed the preview (for example `getWebhook(webhook.id)`). A preview has no real ID now, so `id` stays unknown until the apply. Mirror issue https://github.com/pulumi-labs/pulumi-huggingface/issues/7 (#1590).
- Changing the provider configuration (a rotated token, API key or secret, a new endpoint or region) is an in-place update of the default provider. By default pulumi-go-provider replaced the provider on any config change, and replacing a provider replaces every resource that uses it, so rotating a credential would have deleted and recreated every resource. Each resource's own diff still decides whether it changes. Mirror issue https://github.com/pulumi-labs/pulumi-huggingface/issues/8 (#1591).

## [0.1.1] - 2026-11-05

### Fixed

- Update previews no longer report identifiers as unknown: changing only a mutable input (a `Collection` description, a `Repository` or `Space` `private`) showed `slug`, `url`, `repoId` and `spaceId` as known only after apply, which rippled into every stack output and resource that read them (pulumi-labs/pulumi-huggingface#4). `Collection`, `Repository`, `Space`, `Bucket`, `CollectionItem` and `Webhook` now declare which inputs their outputs depend on.
- Preview no longer fails when an input is not known yet (it is another resource's output): `Space.sdk`, `Webhook.url`/`watched`/`domains`, `RepositoryBranch`/`RepositoryTag` `repoId`/`branch`/`tag`, `Collection.title` and the `CollectionItem` fields were validated as empty values (pulumi-labs/pulumi-huggingface#3). Literal malformed values are still rejected, and unknown ones are checked again at create/update.
- `Repository` and `Space`: the resource ID, `repoId`/`spaceId` and `url` are now always `namespace/name` (`datasets/namespace/name`, for datasets). The Hub's create response carries an internal ObjectId in `id`, and v0.1.0 stored it, so the next refresh, update or delete failed with `invalid resource ID` (pulumi-labs/pulumi-huggingface#2). State written by v0.1.0 cannot be repaired in place (it does not record the name): the error now names the fix, `pulumi state delete` then `pulumi import <type> <name> <namespace>/<name>`.
- `Bucket`: the `name` output is the resolved bucket name (the resource name when the `name` input is unset); it was empty after create, refresh and update (pulumi-labs/pulumi-huggingface#5).

### Added

- Add `qa.yaml`, the runtime test manifest QA reads: credentials, every resource and function with its test plan and cost, and the regressions to re-check. It is copied to the mirror root and changes no provider behavior.

## [0.1.0] - 2026-11-04

### Changed

- `InferenceEndpoint`: new optional `tags` input (create and update payloads follow `huggingface_hub`). Unmanaged while unset; removing tags that were set clears them. No existing property or ID changed.
- `Repository`: new optional `gated` input (`off` default, `auto`, `manual`), applied through the repo settings call together with `private`. No existing property or ID changed.

- `go.mod`/`go.sum` moved from `provider/` to the provider's top level (the monorepo module path now ends in `lib/providers/huggingface`); the Makefile builds from there. No change to the provider binary, schema or SDKs.
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

- New resources `Webhook`, `Collection`, `CollectionItem`, `RepositoryBranch`, `RepositoryTag` and `Bucket`, each with create, read, update (or a documented replace), delete, import by ID alone, deletion-outside-Pulumi handling and an idempotent diff. Import IDs: the webhook ID; the collection slug; `<collection slug>/<item object ID>`; `<models|datasets|spaces>/namespace/name@<ref>` for branches and tags (no prefix means a model); `namespace/name` for buckets. `Webhook.secret` is write-only and secret. `Bucket` replaces on every change (the documented API cannot update one), which deletes its files.
- New lookup functions `getWhoami`, `getInferenceEndpoint`, `listInferenceEndpoints`, `getSpace`, `getRepository`, `getWebhook`, `getCollection`, `getCollectionItem`, `getRepositoryBranch`, `getRepositoryTag` and `getBucket`. `getSpace` returns secret names only: secret values are not supported by the Hugging Face API.
- `research/coverage/huggingface.md`: the coverage inventory of the Hub API, the Inference Endpoints API and the `huggingface_hub` SDK, classifying every operation group (resource, lookup, runtime, deferred, out of scope). The two OpenAPI documents were not downloadable, so shapes were verified against `huggingface_hub`; the inventory lists what stays deferred for that reason (endpoint CIDR lists and custom domains, Job-triggering webhooks, Enterprise groups, scheduled jobs, more repository settings).
- Tests: the fake Hugging Face server now covers webhooks, collections, git refs, buckets, repository `gated`, endpoint tags, listing and whoami, with lifecycle, import, drift, deleted-outside, failure-after-create and lookup tests for every new resource and function, and a schema test that no input is named `version`.

- `templates/inference-endpoint-huggingface-cloud-<lang>` starters (all 6 languages) — scenario template that provisions a dedicated `InferenceEndpoint` plus a Gradio `Space` wired to call it, the production model-serving pattern for HF.
- `templates/` directory with six `pulumi new`-compatible starters (`huggingface-cloud-typescript`, `-python`, `-go`, `-csharp`, `-java`, `-yaml`). Each starter provisions a free-tier Hugging Face model `Repository` plus a Gradio `Space` on `cpu-basic` hardware — a zero-cost onramp to the canonical HF Hub workflow.
- `InferenceEndpoint` resource — provision dedicated GPU/CPU inference endpoints on Hugging Face with full CRUD lifecycle, autoscaling, scale-to-zero, and custom container support (TGI, TEI, vLLM)
- `Space` resource — create and manage Hugging Face Spaces (Gradio, Docker, static) with hardware tier selection, secrets, and environment variables
- `Repository` resource — manage Hugging Face Hub model and dataset repository containers with visibility control
- Provider-level `token` configuration with `HUGGINGFACE_TOKEN` environment variable fallback

