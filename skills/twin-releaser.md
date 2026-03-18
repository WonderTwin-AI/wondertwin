# SKILL: WonderTwin Twin Releaser

## Purpose

Take a code-complete twin from "tests pass on a branch" to "in customers' hands with marketing aware." This skill covers the gap between the generator/extender completing code and the twin being usable by customers — CI integration, release workflow, registry publication, issue closure with messaging, and the marketing handoff document.

This is the final lifecycle skill. It runs after the generator (or extender) has produced working code and the conformance suite passes.

## When to Use

- A new twin has been built and merged to main
- An existing twin has been extended with significant new coverage and is ready for a version bump
- A fintech twin's gap register has been fully resolved

## Prerequisites

1. Twin code is on `main` and passes `go test ./twin-{name}/...`
2. `go build ./twin-{name}/cmd/twin-{name}` succeeds
3. Conformance suite passes: `wt conformance` (10 checks)
4. Engine conformance passes (if applicable): e.g., `go test ./twinkit/ledger/accounting/conformance/...`
5. `twin-manifest.json` and `provenance.json` are final with accurate coverage numbers

## Process

### Step 1: CI and Build Integration

Verify the twin is in all build lists:

**1a. `.github/workflows/ci.yml`** — "Build all twins" step:
```bash
grep "twin-{name}" .github/workflows/ci.yml || echo "MISSING from CI"
```
Add if missing. All twins must be built by CI on every push to main.

**1b. `Makefile`** — `TWINS` variable:
```bash
grep "{name}" Makefile || echo "MISSING from Makefile"
```
Add to the `TWINS` list. This enables `make build-twins` to include the new twin.

**1c. `go.work`** — workspace:
```bash
grep "twin-{name}" go.work || echo "MISSING from go.work"
```

**1d. Full workspace verification:**
```bash
go build ./...
go test ./... -count=1 -timeout 120s
```

### Step 2: Twin Metadata

Verify all metadata files are present and accurate:

**2a. `twin-{name}/twin.json`:**
```json
{
  "name": "{name}",
  "description": "{One-line description of what API this simulates}",
  "category": "{category}",
  "sdk": {
    "package": "{sdk_import_path or empty}",
    "version": "{sdk_version or empty}"
  },
  "default_port": {port}
}
```

**2b. `twin-{name}/twin-manifest.json`** — verify:
- `coverage.resources_implemented` lists all implemented resources
- `coverage.resources_not_implemented` lists known gaps
- `coverage.estimated_coverage_pct` is realistic
- `service_surface.resource_count` matches actual endpoint count

**2c. `twin-{name}/provenance.json`** — verify:
- `sources` lists all documentation and SDK references used
- `scope.implemented` and `scope.not_implemented` are accurate
- `api_version` is pinned to the specific version researched

### Step 3: Release the Twin

**3a. Trigger the twin release workflow:**

```bash
gh workflow run release-twin.yml -f twin={name} -f version=0.1.0
```

This will:
1. Create a git tag: `twin-{name}-v0.1.0`
2. Build cross-platform binaries (darwin-amd64, darwin-arm64, linux-amd64, linux-arm64)
3. Compute SHA-256 checksums
4. Create a GitHub Release on the registry repo
5. Update `registry.json` with binary URLs and checksums
6. Validate: install the twin from registry and run conformance

**3b. Verify the release:**

```bash
# Check the tag exists
git tag -l "twin-{name}-v*"

# Verify it's installable
wt install {name}@0.1.0

# Run conformance against the installed binary
wt conformance $(which twin-{name})
```

### Step 4: Close the GitHub Issue

Every twin has a tracking issue. Close it with a structured comment that serves as the announcement to anyone watching the issue.

**Issue closure comment template:**

```markdown
## twin-{name} v{version} is live 🎉

**What**: Behavioral twin for the {Platform} API — simulates {brief description of what it does}.

**Install**:
```
wt install {name}@{version}
```

**Port**: {port} | **Prefix**: `{api_prefix}`

### What's covered
{Bulleted list of implemented features — copy from provenance.json scope.implemented}

### What's not covered (yet)
{Bulleted list from provenance.json scope.not_implemented, or "See twin-manifest.json for full coverage details."}

### Key behavioral fidelity
{3-5 bullets on the platform-specific behaviors the twin faithfully simulates — the things that make this more than a dumb mock. Examples: SyncToken optimistic locking, Balance-derived invoice status, HMAC-SHA256 webhook signing, SQL-like query language.}

### Quick start
```json
// wondertwin.json
{
  "twins": {
    "{name}": {
      "version": "{version}",
      "port": {port}
    }
  }
}
```
```bash
wt install
wt up
wt status
curl http://localhost:{port}/admin/health
```

### Research & provenance
- Research archive: `wondertwin-docs/research/{platform}/`
- API version: {api_version}
- Sources: {brief list of key sources}

Built with `twinkit/` shared libraries{, including twinkit/ledger/accounting engine if fintech}.
```

Close the issue with this comment. The comment is the primary announcement artifact — it shows up in email notifications for all watchers, in the issue history, and is linkable.

### Step 5: Marketing Handoff Document

Create `wondertwin-docs/releases/twin-{name}-v{version}-handoff.md` with the following structure. This is the document marketing uses to create landing page content, blog posts, social posts, and outbound messaging.

```markdown
# Marketing Handoff: twin-{name} v{version}

**Release date**: {date}
**Category**: {category}
**Target ICP**: {which ICP from the ICP framework — Builder, API Provider, or Agentic Platform}

---

## 1. Positioning

### One-liner
{Single sentence a developer would use to describe this to a colleague. Must be copy-pasteable into a Slack message.}

### Elevator pitch (30 seconds)
{2-3 sentences for a blog post intro or social post. Focus on the problem, not the feature set.}

### What it replaces
{What developers did before. Be specific to this platform:}
- **Sandbox dependency**: {Does the platform offer free sandboxes? If yes, what's wrong with them (rate limits, stale data, shared state, slow provisioning)? If no, that's the killer point — "You literally cannot test this without a production account."}
- **Hand-rolled mocks**: {What do developers typically mock? Is it painful? Do mocks drift?}
- **Skipping tests entirely**: {How common is it for teams to skip integration tests for this platform and find out in production?}

---

## 2. Use Cases

### Sales demo scenarios
{Concrete scenarios a salesperson can walk a prospect through. Each should be completable in under 5 minutes.}

**Scenario A: "First integration test in 60 seconds"**
{Show: install twin, wt up, curl a create endpoint, verify response matches real API shape. Point: zero signup, zero config, instant feedback loop.}

**Scenario B: "CI pipeline that tests your {Platform} integration"**
{Show: wondertwin.json in a repo, GitHub Actions workflow that runs wt up → wt test → wt down. Point: integration tests that run on every PR, no external dependencies, no flaky sandbox.}

**Scenario C: "Catch the bug before production"**
{Show: a realistic failure scenario specific to this platform. Examples:
- Xero: SyncToken conflict when two processes update the same invoice
- QBO: Query returns all entities because WHERE filter wasn't applied
- Stripe: Payout fails because account balance is insufficient
Point: the twin simulates the exact error the real API would return.}

### Software development use cases
{How developers use this twin day-to-day. Be specific.}

- **Local development loop**: Developer writes integration code → runs twin locally → iterates without network calls, rate limits, or API keys. Typical cycle: edit code, `curl localhost:{port}`, verify — under 1 second.
- **SDK validation**: Plug the real {Platform} SDK into the twin's base URL. If the SDK works against the twin, it works against production. No SDK wrapper or adapter needed.
- **Webhook development**: Fire webhook events on demand (`/admin/webhooks/flush`). No waiting for real events to trigger. Test your webhook handler against realistic signed payloads.
- **Seed data for demos**: Load a complete business state via `POST /admin/state`. Sales demos, investor demos, QA environments — all reproducible from a JSON file.
- **State inspection**: `GET /admin/state` dumps the full twin state. `GET /admin/requests` shows every request the twin received. Debug integration issues by reading the twin's memory, not guessing from API responses.

### Agent development use cases
{How AI agents and agentic platforms use this twin.}

- **Safe iteration**: Agents can call the twin thousands of times without hitting rate limits, incurring API costs, or creating real transactions. The agent iterates against the twin, then runs the validated workflow against the real API.
- **MCP server**: `wt mcp` exposes the twin as a tool for AI agents via the Model Context Protocol. The agent can start twins, create test data, run scenarios, and inspect state — all programmatically.
- **Deterministic replay**: Reset twin state, seed known data, run the agent, verify outcomes. Reproducible agent testing without external API variability.

### Maintenance use cases
{How the twin saves time after the initial integration is built.}

- **Regression detection**: Run integration tests in CI on every commit. Catch regressions in your {Platform} integration code before they ship.
- **SDK upgrade validation**: Upgrade the {Platform} SDK, run your test suite against the twin. If tests pass, the upgrade is safe. No need to manually verify against a sandbox.
- **Incident reproduction**: Customer reports a {Platform}-related bug. Seed the twin with their state, reproduce locally, fix, verify — without touching production data.
- **Onboarding**: New team members run the full integration test suite locally on day 1. No sandbox account provisioning, no API key sharing, no "ask Dave for the test credentials."

---

## 3. Economics

### Time cost: manual integration testing vs. twins

| Activity | Manual (per year) | With WonderTwin |
|----------|------------------|-----------------|
| Sandbox provisioning & credential management | 8-16 hours | 0 (no credentials needed) |
| Writing and maintaining hand-rolled mocks | 40-80 hours | 0 (twin replaces mocks) |
| Debugging flaky integration tests (sandbox timeouts, rate limits, shared state) | 20-40 hours | 0 (local, deterministic) |
| Manual smoke testing before releases | 16-32 hours | Automated in CI |
| Reproducing {Platform}-related production incidents | 8-16 hours | Minutes (seed + reproduce locally) |
| Onboarding new developers to the integration | 4-8 hours per person | `wt up` and read the test suite |
| **Total estimated annual savings** | **~100-200 hours per integration** | — |

{Adjust these ranges based on the specific platform. Some platforms have especially painful sandboxes (NetSuite), some have no sandbox at all (many fintech APIs), some have aggressive rate limits (GitHub, QBO). Call out the platform-specific pain.}

### Cost avoidance

- **No sandbox fees**: Some platforms charge for sandbox access or test environments. The twin is free.
- **No API call costs**: Development and CI test runs hit the twin, not the real API. For metered APIs, this is direct cost savings.
- **Faster incident resolution**: Hours of production debugging → minutes of local reproduction. MTTR reduction for {Platform}-related incidents.
- **Reduced production incidents**: Integration tests that actually run (vs. skipped because the sandbox is down) catch bugs before customers see them.

---

## 4. Platform Details

### Surface coverage
- **API endpoints**: {count}
- **Entity types**: {count}
- **Reports**: {count, if applicable}
- **Webhook events**: {count}
- **Auth model**: {e.g., "OAuth 2.0 Bearer token + realmId in URL path"}
- **Query capabilities**: {e.g., "SQL-like query language with WHERE, ORDERBY, pagination"}

### Behavioral fidelity highlights
{The 3-5 platform-specific behaviors that demonstrate this is a simulation, not a dumb mock. These are the "wow" details for technical blog posts and sales demos.}

### What the real platform charges for testing
{What does the platform charge for sandbox/test access? Is it free? Time-limited? Feature-limited? This is competitive intelligence for outbound.}

### Who uses this platform
{Market data on the platform's customer base. How many companies use {Platform}? What industries? What size? This helps marketing target outbound.}

---

## 5. Demo Walkthrough

{A concrete, copy-pasteable curl-based demo that shows the twin's value in 10 commands or less. Should be completable in under 3 minutes. Include the `wt up` and health check.}

```bash
# Install and start
wt install {name}@{version}
echo '{"twins": {"{name}": {"version": "{version}", "port": {port}}}}' > wondertwin.json
wt up

# Verify
curl -s localhost:{port}/admin/health | jq

# {3-5 API calls that demonstrate a realistic workflow}
# ...

# Inspect state
curl -s localhost:{port}/admin/state | jq '.{entity_name} | length'

# Clean up
wt down
```

---

## 6. Content Hooks

{Suggested angles for each content type. Be specific — these should be assignable.}

### Blog post ideas
- "{Title}" — {One paragraph pitch for the post}
- "{Title}" — {One paragraph pitch}

### Social posts
- Twitter: "{Draft tweet — under 280 chars}"
- LinkedIn: "{Draft post — 2-3 sentences}"

### Outbound hooks
- SDR email subject: "{Subject line}"
- SDR email body hook: "{One sentence that establishes relevance — e.g., 'I noticed you're using Xero's API. We built a local Xero simulator that lets you test your integration without an Xero account.'}"

### Partner/ecosystem angles
- {Is there an opportunity to engage the platform itself? Developer relations? Marketplace listing? Community forum post?}

---

## 7. Competitive Context

{Who else provides {Platform} testing tools? What's the gap?}

- **Platform's own sandbox**: {Does one exist? Limitations?}
- **Generic API mocking tools** (Prism, WireMock, MockServer): {Why is a behavioral twin better than schema-based mocking for this platform?}
- **Other simulators**: {Has anyone else built a {Platform} simulator? Search GitHub for prior art — reference the research artifacts.}

---

## 8. Assets Checklist for Marketing

- [ ] Landing page: catalog entry with one-liner, install command, endpoint count
- [ ] Blog post: announcement + technical deep dive on behavioral fidelity
- [ ] Social: Twitter thread + LinkedIn post
- [ ] Twin catalog update on wondertwin.ai
- [ ] Outbound email template targeting {Platform} users
- [ ] Demo recording (optional): 3-minute walkthrough of demo scenario
- [ ] Changelog entry on wondertwin.ai

## Links
- GitHub: `twin-{name}/` in the monorepo
- Research: `wondertwin-docs/research/{platform}/` (if applicable)
- Twin manifest: `twin-{name}/twin-manifest.json`
- Provenance: `twin-{name}/provenance.json`
- Feature gap register: `wondertwin-docs/research/{platform}/feature-gap-register.md` (if applicable)
```

### Step 6: Notify the Team

After the issue is closed and the handoff document is written:

1. Commit the handoff document to `wondertwin-docs/releases/`
2. Link the handoff document in the closed issue as a follow-up comment
3. If there's a Slack channel or team notification mechanism, post a brief "twin-{name} v{version} shipped" message with a link to the issue

---

## Checklist

Before considering a twin release complete:

- [ ] CI builds the twin (`ci.yml` twin list)
- [ ] Makefile includes the twin (`TWINS` variable)
- [ ] `go.work` includes the twin
- [ ] `twin.json` is accurate
- [ ] `twin-manifest.json` coverage is final
- [ ] `provenance.json` scope is final
- [ ] Twin release workflow ran successfully
- [ ] Twin is installable via `wt install {name}@{version}`
- [ ] Conformance passes on the released binary
- [ ] GitHub issue closed with structured comment
- [ ] Marketing handoff document committed to `wondertwin-docs/releases/`

---

## Common Mistakes

- **Releasing before CI is updated.** The twin passes local tests but CI doesn't build it. Next person to touch the repo breaks the twin without knowing.
- **Closing the issue without a structured comment.** A bare "done" or auto-close via PR merge gives watchers no useful information. The issue comment IS the announcement.
- **Forgetting the Makefile.** `make build-twins` is used in local development. If the twin isn't in `TWINS`, developers won't build it.
- **Stale coverage numbers in the manifest.** The manifest says 15% coverage but the twin actually implements 85%. Update after every gap fix.
- **No marketing handoff.** Engineering ships a twin and marketing finds out weeks later. The handoff document is the bridge.
