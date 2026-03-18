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
**Target ICP**: {which ICP from the ICP framework this twin serves — Builder, API Provider, or Agentic Platform}

## One-liner
{Single sentence a developer would use to describe this to a colleague.}

## Elevator pitch (30 seconds)
{2-3 sentences for a blog post intro or social post. Focus on what problem this solves, not what it is.}

## What it replaces
{What were developers doing before this twin existed? Mocking by hand? Using a flaky sandbox? Skipping tests entirely? Be specific to this platform.}

## Key differentiators
{3-5 bullets on what makes this twin valuable. Not features — value. Example: "Tests your Xero integration without an Xero account" not "Has 27 API endpoints."}

## Platform stats
- **API endpoints**: {count}
- **Entity types**: {count}
- **Reports**: {count, if applicable}
- **Webhook events**: {count}
- **Auth model**: {brief — e.g., "Bearer token + Xero-Tenant-Id header"}

## Behavioral fidelity highlights
{The 3-5 platform-specific behaviors that demonstrate this is a real simulation, not a mock. These are the "wow" details for technical blog posts.}

## Demo scenario
{A concrete curl-based walkthrough that someone could copy-paste to see the twin in action. 5-10 steps. Create entity → query → update → verify state.}

## Content hooks
{Suggested angles for blog posts, tweets, or outreach:}
- Blog: "{Title idea}" — {one-line pitch}
- Tweet: "{Draft tweet}"
- Outreach: "{One-line for SDR emails targeting companies that use {Platform}}"

## Competitive context
{Who else provides {Platform} testing tools? What's the gap? Reference the research artifacts if available.}

## Assets needed from marketing
- [ ] Landing page section for this twin (catalog entry)
- [ ] Blog post announcing the twin
- [ ] Social posts (Twitter, LinkedIn)
- [ ] Update to twin catalog on wondertwin.ai
- [ ] Outbound email template for companies using {Platform}

## Links
- GitHub: `twin-{name}/` in the monorepo
- Research: `wondertwin-docs/research/{platform}/` (if applicable)
- Twin manifest: `twin-{name}/twin-manifest.json`
- Provenance: `twin-{name}/provenance.json`
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
