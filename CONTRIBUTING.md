# Contributing to WonderTwin

WonderTwin is an open-source catalog of behavioral API twins. The most impactful way to contribute is to **add a twin for an API you use**.

## Request a Twin

Don't see the API you need? [Open a twin request →](https://github.com/wondertwin-ai/wondertwin/issues/new?template=twin-request.yml)

Tell us which service, which endpoints matter, and how you're using the API. This helps us (and other contributors) prioritize what to build next.

## Build a Twin

Every twin is a standalone Go binary that behaviorally clones a third-party API. Twins maintain state in memory, enforce business logic, fire webhooks, and expose the same endpoints the real API does — so official SDKs work against them unmodified.

### What you need

- Go 1.21+
- The target API's public docs or SDK reference
- An AI coding agent (recommended — most twins are generated in 2-4 hours)

### The process

1. **Pick an API.** Check the [twin request issues](https://github.com/wondertwin-ai/wondertwin/issues?q=is%3Aissue+label%3Atwin-request) for popular requests, or build something you need yourself.

2. **Scaffold the twin.** Every twin follows the same structure:

   ```
   twin-{name}/
   ├── cmd/twin-{name}/main.go
   ├── internal/
   │   ├── api/
   │   │   ├── router.go
   │   │   └── handlers_{resource}.go
   │   └── store/
   │       ├── memory.go
   │       └── types.go
   ├── go.mod
   └── go.sum
   ```

3. **Use the shared libraries.** All twins import [`twinkit`](https://github.com/wondertwin-ai/twinkit) for server scaffolding, in-memory storage, admin endpoints, webhooks, and test helpers:

   ```
   go get github.com/wondertwin-ai/twinkit@latest
   ```

4. **Follow the skill guide.** The detailed, step-by-step build guide lives at [`skills/twin-generator.md`](skills/twin-generator.md). It covers everything: router setup, store design, handler implementation, webhook wiring, seed data, testing, and common pitfalls. If you're using an AI coding agent, feed it this file along with the target API docs.

5. **Test locally.** Build the binary and wire it into a `wondertwin.yaml`:

   ```yaml
   twins:
     my-api:
       binary: ./path/to/twin-my-api
       port: 9020
   ```

   ```bash
   wt up
   wt status
   wt test    # if you have test scenarios
   ```

   No registry, no license key, no network. Fully offline.

6. **Submit a PR.** Open a pull request to this repo with your twin. Include:
   - The twin source code in `twin-{name}/`
   - A brief description of which endpoints are covered
   - Example seed data if applicable

### Quality bar

Every twin must:

- **Start and respond to health checks.** `GET /admin/health` returns 200.
- **Implement the admin API.** Reset, state snapshot/load, fault injection, and time simulation — all provided by `twinkit/admin`, so this is automatic if you wire it up.
- **Work with the official SDK.** Point the SDK at your twin's port and run real operations. If the SDK doesn't work, the twin isn't done.
- **Maintain state correctly.** Create a resource, retrieve it, verify the data matches. Reset, verify it's gone.
- **Handle common error cases.** Missing required fields, duplicate IDs, not-found lookups. Return the same error format the real API uses.

### Using an AI coding agent

This is the recommended path. The twin generator skill (`skills/twin-generator.md`) was designed to be fed directly to an AI agent alongside the target API's documentation. The skill handles the architecture decisions — your agent just needs to implement the endpoints.

Typical workflow:
1. Give your agent the skill guide + the API docs URL
2. The agent scaffolds the project, implements handlers, writes tests
3. You review, build, test locally with `wt up`
4. Submit the PR

## Other Contributions

### Bug fixes and improvements

For bugs or improvements to the CLI, shared libraries, or existing twins:

1. Fork the repo
2. Create a branch (`fix/describe-the-fix` or `feat/describe-the-feature`)
3. Make your changes
4. Open a PR with a clear description of what changed and why

### Test scenarios

Each twin can have YAML test scenarios in `scenarios/` that validate behavior using `wt test`. Adding test coverage for existing twins is valuable — especially edge cases and error paths.

## Questions?

Open an issue or start a discussion. We're happy to help scope a twin or pair on tricky API behaviors.
