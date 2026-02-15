# Contributing to WonderTwin

Welcome, and thank you for your interest in WonderTwin. This project is an open-source catalog of behavioral service twins -- local, in-memory replicas of third-party APIs that let official SDKs work against them unmodified. The most impactful way to contribute is to **add a twin for a service you use**.

This guide covers everything you need to know to contribute a new twin or improve an existing one.

---

## Table of Contents

- [Request a Twin](#request-a-twin)
- [Build a Twin](#build-a-twin)
  - [The SDK-First Approach](#the-sdk-first-approach)
  - [What You Need](#what-you-need)
  - [Directory Structure](#directory-structure)
  - [Step-by-Step Process](#step-by-step-process)
  - [Using the Twin Generator Skill](#using-the-twin-generator-skill)
  - [The Manifest and Provenance Files](#the-manifest-and-provenance-files)
  - [Quality Checklist](#quality-checklist)
  - [Reporting Quirks](#reporting-quirks)
- [Other Contributions](#other-contributions)
- [Questions](#questions)

---

## Request a Twin

Don't see the service you need? [Open a twin request](https://github.com/wondertwin-ai/wondertwin/issues/new?template=twin-request.yml).

Tell us which service, which SDK operations matter, and how you use the service in your tests. This helps us (and other contributors) prioritize what to build next.

---

## Build a Twin

Every twin is a standalone Go binary that behaviorally clones a third-party service's API. Twins maintain state in memory, enforce business logic, fire webhooks, and expose the same endpoints the real service does -- so official SDKs work against them unmodified.

### The SDK-First Approach

WonderTwin twins are **SDK-first, not API-first**. This is the single most important design principle:

- **Target SDK compatibility**, not just OpenAPI conformance. The goal is for the official SDK client library to work against the twin without any modification -- just point it at localhost.
- **Study the SDK**, not just the API docs. SDKs often expect specific response shapes, headers, error formats, and pagination patterns that are not fully captured in OpenAPI specs.
- **Test with the real SDK**. If the SDK does not work against your twin, the twin is not done. Passing an OpenAPI validator is necessary but not sufficient.
- **Match behavioral details**. ID formats, timestamp formats, default values, sort orders, envelope structures -- these all matter because the SDK depends on them.

This means when you build a twin for Stripe, you run the `stripe-go` SDK against it. When you build a twin for Resend, you run the `resend-go` SDK against it. The SDK is the spec.

### What You Need

- Go 1.21+
- The target service's public docs or SDK reference
- An AI coding agent (recommended -- most twins are generated in 2-4 hours)

### Directory Structure

Every twin follows the same layout. A copyable template is available at [`docs/TWIN_TEMPLATE/`](docs/TWIN_TEMPLATE/):

```
twin-{name}/
├── cmd/twin-{name}/main.go          # Entry point: parse flags, wire up stores, serve
├── internal/
│   ├── api/
│   │   ├── router.go                # Handler struct, Routes(), auth middleware
│   │   ├── handlers_{resource}.go   # One file per API resource group
│   │   └── handlers_test.go         # Handler tests using testutil.TwinClient
│   └── store/
│       ├── memory.go                # MemoryStore implementing admin.StateStore
│       └── types.go                 # Domain structs with JSON tags
├── twin-manifest.json               # Required: describes the twin's capabilities
├── provenance.json                  # Required: records how the twin was generated
├── go.mod                           # Module: github.com/wondertwin-ai/wondertwin/twin-{name}
└── go.sum
```

**Required files:**

| File | Purpose |
|------|---------|
| `twin-manifest.json` | Describes the twin, its SDK target, service surface, coverage, and generation method. Must validate against [`schemas/twin-manifest.schema.json`](schemas/twin-manifest.schema.json). |
| `provenance.json` | Records how the twin was generated, what sources were used, and when. Must validate against [`schemas/provenance.schema.json`](schemas/provenance.schema.json). |
| `cmd/twin-{name}/main.go` | Entry point that wires up the store, API handlers, and admin handlers. |
| `internal/api/router.go` | Defines the `Handler` struct, `Routes()` method, and auth middleware. |
| `internal/api/handlers_*.go` | One file per resource group with the actual endpoint logic. |
| `internal/api/handlers_test.go` | Tests using `testutil.TwinClient` for all endpoints. |
| `internal/store/types.go` | Domain types that match the real service's response shapes. |
| `internal/store/memory.go` | `MemoryStore` with `Snapshot()`, `LoadState()`, and `Reset()` methods. |

### Step-by-Step Process

1. **Pick a service.** Check the [twin request issues](https://github.com/wondertwin-ai/wondertwin/issues?q=is%3Aissue+label%3Atwin-request) for popular requests, or build something you need yourself.

2. **Copy the template.** Start from `docs/TWIN_TEMPLATE/`:
   ```bash
   cp -r docs/TWIN_TEMPLATE twin-{name}
   ```
   Then find-and-replace `TEMPLATE` with your service name and update the placeholder values.

3. **Use the shared libraries.** All twins import `twinkit` for server scaffolding, in-memory storage, admin endpoints, webhooks, and test helpers:
   ```bash
   go get github.com/wondertwin-ai/twinkit@latest
   ```
   Within the monorepo, the `go.mod` uses a replace directive:
   ```
   replace github.com/wondertwin-ai/wondertwin/twinkit => ../twinkit
   ```

4. **Implement the handlers.** For each SDK resource:
   - Study how the SDK client calls the API (request shape, headers, URL patterns).
   - Implement the handler to return responses the SDK expects.
   - Match error formats exactly -- SDKs parse error responses.
   - Store state in the `MemoryStore` so create/read/update/delete cycles work.

5. **Write tests.** Use `testutil.TwinClient` for clean HTTP test helpers:
   ```go
   srv, tc := setupTwin(t)
   resp := tc.DoWithHeaders("POST", "/v1/resources", body, authHeaders)
   resp.AssertStatus(201)
   ```

6. **Fill in the manifest and provenance files.** See [The Manifest and Provenance Files](#the-manifest-and-provenance-files) below.

7. **Test locally.** Build the binary and wire it into a `wondertwin.yaml`:
   ```yaml
   twins:
     my-service:
       binary: ./path/to/twin-my-service
       port: 9020
   ```
   ```bash
   wt up
   wt status
   wt conformance   # Verify admin API conformance
   ```
   No registry, no license key, no network. Fully offline.

8. **Submit a PR.** Use the [new twin PR template](.github/PULL_REQUEST_TEMPLATE/new_twin.md) and work through the checklist.

### Using the Twin Generator Skill

The fastest way to build a twin is with the twin generator skill and an AI coding agent. The skill is documented in [`skills/twin-generator.md`](skills/twin-generator.md) and covers the full process: project setup, router design, store design, handler implementation, webhook wiring, testing, and common pitfalls.

**Typical workflow:**

1. Give your agent the skill guide (`skills/twin-generator.md`) plus the target service's API docs or SDK reference URL.
2. The agent scaffolds the project, implements handlers, and writes tests.
3. You review the output, build it, and test locally with `wt up`.
4. Fill in `twin-manifest.json` and `provenance.json`.
5. Submit the PR.

The skill is designed to produce twins that conform to the standard structure and quality bar from the start.

### The Manifest and Provenance Files

#### `twin-manifest.json`

Every twin must include a manifest file that describes its capabilities. The schema is at [`schemas/twin-manifest.schema.json`](schemas/twin-manifest.schema.json).

Key fields:

| Field | Description |
|-------|-------------|
| `twin` | Machine-readable identifier (e.g., `resend`, `stripe`) |
| `display_name` | Human-readable name (e.g., `Resend`, `Stripe`) |
| `category` | Service category (e.g., `email`, `payments`, `auth`) |
| `sdk_target.primary` | The SDK this twin targets: package, language, version, repo URL |
| `service_surface` | Auth pattern, webhook support, resource count |
| `coverage` | Which resources are implemented vs. not yet implemented |
| `generation` | How the twin was built (skill, manual, or other) |

#### `provenance.json`

Every twin must include a provenance file that records how it was generated. The schema is at [`schemas/provenance.schema.json`](schemas/provenance.schema.json).

This file tracks:
- Which SDK version was targeted
- When the twin was generated
- What sources were used (OpenAPI spec, SDK analysis, Arazzo workflows)
- Whether fallback methods were needed

Provenance tracking ensures reproducibility and makes it possible to detect when a twin needs updating because the upstream SDK has changed.

### Quality Checklist

Before submitting your PR, verify:

- [ ] **`twin-manifest.json` is present and valid.** Validates against the schema.
- [ ] **`provenance.json` is present and valid.** Validates against the schema.
- [ ] **Standard directory structure is followed.** `cmd/`, `internal/api/`, `internal/store/`.
- [ ] **Admin API conformance passes.** Run `wt conformance` -- health, reset, state snapshot/load, fault injection, and time simulation must all work.
- [ ] **Handler tests pass.** `go test ./...` in the twin directory.
- [ ] **At least one seed data example exists.** Either as a JSON file or inline in tests.
- [ ] **SDK client works.** Point the official SDK at the twin and run real operations.
- [ ] **Error formats match.** The SDK parses error responses -- yours must match the real service.
- [ ] **State is correct.** Create a resource, retrieve it, verify the data matches. Reset, verify it is gone.
- [ ] **README documents coverage and limitations.** What works, what does not, what is known to differ.

### Reporting Quirks

Real-world services have undocumented behaviors, inconsistencies, and edge cases that do not appear in their API docs. When you discover these while building a twin, please document them:

1. **In the twin's README**, add a "Known Quirks" section describing the behavior.
2. **In code comments**, annotate the handler where you implement the quirk with a `// Quirk:` comment explaining what the real service does and why.
3. **In the twin-manifest.json**, if the quirk means the twin intentionally differs from documented behavior, note it in the coverage section.

These quirk reports are valuable to the entire community. They capture hard-won knowledge about how services actually behave versus how they are documented.

---

## Other Contributions

### Bug Fixes and Improvements

For bugs or improvements to the CLI, shared libraries, or existing twins:

1. Fork the repo
2. Create a branch (`fix/describe-the-fix` or `feat/describe-the-feature`)
3. Make your changes
4. Open a PR with a clear description of what changed and why

### Test Scenarios

Each twin can have YAML test scenarios in `scenarios/` that validate behavior using `wt test`. Adding test coverage for existing twins is valuable -- especially edge cases and error paths.

### Documentation

Improvements to this guide, the skill guide, or individual twin READMEs are always welcome.

---

## Questions?

Open an issue or start a discussion. We are happy to help scope a twin or pair on tricky service behaviors.
