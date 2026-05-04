---
skill: twin-researcher
skill_version: "2.0"
schemas:
  provenance.schema.json: "1.1"
---

# SKILL: Twin Researcher (community)

## Purpose

Research a target platform well enough to build a community twin against it,
and produce structured, provenanced research artifacts that pin the twin to
specific versions of the platform's API and SDK.

This skill is the entry point of the community twin lifecycle. It precedes the
community twin generator and feeds into the twin maintainer when platform
releases require updates.

Research starts from inquiry, not confirmation. The goal is to build a mental
model of the target system complete enough to simulate its observable behavior
— and to know where the model is uncertain.

## Audience and tier

This is the **public, MIT-licensed** researcher skill, intended for community
contributors. Two cases this skill serves:

1. A developer building a twin of commercial software they integrate with
   (e.g., a Stripe twin for testing their payment flow without hitting Stripe's
   sandbox).
2. A vendor building a twin of their own software so customers can develop
   integrations offline.

The skill scope is "how to research a twin from public information." It covers
discovery from SDKs, OpenAPI specs, and public docs; behavior observation
methods that respect ToS; quirk identification from documented edge cases and
community signal; and the artifact set the community twin generator needs.

Internal operator concerns (market analysis, vendor prioritization, customer-
demand signal integration, internal pipelines) are out of scope for this
skill. Operators running an internal twin pipeline use a separately-maintained,
private flavor of this skill.

## Inputs

- **Platform name** — the target platform (e.g., `quickbooks-online`, `mercury`, `stripe`)
- **Category** — functional category (e.g., `accounting`, `banking-as-a-service`, `payments`)
- **Research scope** — one of:
  - `candidate` — minimum viable research for go/no-go (Phases 0, 1, 2, 4, 7, 8)
  - `build-ready` — full research for first implementation phase (all phases applicable to the planned roadmap phase)
  - `targeted` — investigate a specific area (specify which Section 1 questions)
- **Known context** (optional) — anything you already know: SDK names, doc URLs, prior research

## Output structure

All research artifacts live under `wondertwin-docs/research/{platform}/`:

```
wondertwin-docs/research/{platform}/
├── RESEARCH-STATUS.md              # Status, confidence + gap list with EV, next steps
├── system-model.md                 # Section 1 questions with confidence scores
├── record-type-catalog.json        # Inventory of types, fields, operations, relationships
├── compatibility-path.md           # SDK landscape, target version, build strategy
├── event-model.md                  # Webhook/event classification (when applicable)
├── release-timeline.json           # Historical/projected releases, deprecation dates
├── observations.json               # Notable API behaviors with provenance
├── source-catalog.json             # Index of all discovered and archived sources
├── provenance-log.jsonl            # Append-only log of research activities
├── feasibility.md                  # Go/no-go with rationale
├── architecture-spec.md            # Technical design for twin (incl. engine analysis)
├── roadmap.json                    # Phased build plan
├── quirk-hypotheses.json           # Behavioral quirks from SDK audit (15-30 items)
└── archive/
    ├── official-docs/{date}/
    ├── schemas/{api-version}/
    ├── sdks/{sdk-name}@{version}.md
    ├── release-notes/{version}.md
    ├── community/{source-id}.md
    └── observations/{date}-{description}.md
```

## Process

### Phase 0: Initialize and meta-research

Start here regardless of scope. Understanding what sources exist determines
how to investigate everything else.

**1. Initialize the directory:**

```bash
platform="{platform}"  # lowercase, hyphenated
base="wondertwin-docs/research/${platform}"
mkdir -p "${base}/archive/"{official-docs,schemas,sdks,release-notes,community,observations}
```

**2. Initialize tracking files:**

`source-catalog.json`:
```json
{
  "platform": "{platform}",
  "display_name": "{Platform Display Name}",
  "category": "{category}",
  "research_initiated": "{ISO 8601}",
  "sources": []
}
```

`provenance-log.jsonl` — first entry:
```json
{"timestamp": "{ISO 8601}", "action": "research_initiated", "scope": "{candidate|build-ready|targeted}"}
```

`RESEARCH-STATUS.md` — see template below ("RESEARCH-STATUS template").

**3. Discover official sources:**

- Search for the platform's developer documentation portal
- Identify the documentation structure (REST reference, guides, tutorials, changelog)
- Search for OpenAPI/Swagger specs at `{api-domain}/openapi.json`, GitHub repos, doc pages
- Search for WSDL, GraphQL schema, or other machine-readable definitions
- Fetch and archive everything found to `archive/{schemas|official-docs}/`

**4. Search for deprecation and lifecycle information:**

This is critical and easily missed. Always search for:

- "{platform} API deprecation"
- "{platform} SOAP deprecated" / "REST deprecated"
- "{platform} API sunset timeline"
- "{platform} API end of life"
- Release notes for "deprecation", "sunset", "end of support"
- Developer blogs for migration announcements
- Support bulletins for breaking changes

**Why:** choosing a deprecated API wastes effort. Deprecation timelines
validate API target choice and inform migration planning.

**5. Survey the SDK landscape:**

Search major package registries:
- Go: `pkg.go.dev` search
- Python: PyPI search
- Node: npmjs.com search
- Ruby: rubygems.org search

For each SDK found, record: name, maintainer (official vs. community), version,
last updated, stars/downloads. Assess the SDK spectrum position (multiple
canonical, single canonical, canonical + community, community only, API docs +
schema, API docs only, restricted access).

**6. Identify community sources:**

- Platform's developer forum, Slack/Discord, Stack Overflow tag
- iPaaS connector documentation (Workato, Celigo, Tray.io) — often documents auth quirks
- GitHub for existing mocks/simulators of this platform (prior art)

**7. Catalog every source:**

For each, append to `source-catalog.json` sources array:

```json
{
  "source_id": "{unique-id}",
  "source_type": "official_docs|sdk|openapi|community|ipaas|prior_art",
  "url": "{url}",
  "access": "public|gated|unavailable",
  "reliability": "high|medium|low",
  "discovered_at": "{ISO 8601}",
  "archived": true,
  "archive_path": "archive/{subdir}/{filename}",
  "notes": ""
}
```

**Update Section 1.7 confidence in `RESEARCH-STATUS.md`.**

### Phase 1: System identity (Section 1.1)

**Must answer:**
- What does this system do? What problem does it solve?
- What is the conceptual data model? What entities does it think in?
- What are the system's boundaries?
- Who uses it? (End users, developers, integrators)

**Sources:** Official "what is" / overview pages, Wikipedia, vendor marketing,
industry analysis.

**Output:** `system-model.md` Section 1.1 with provenance citations.

### Phase 2: API surface (Section 1.2)

**Must answer:**
- What API protocols exist? (REST, SOAP, GraphQL, gRPC, webhooks)
- For each: status (active, deprecated, emerging)
- Do protocols share underlying state?
- URL structure, versioning model, serialization formats

**Method:** Doc tree crawl of API reference. If OpenAPI spec was found in
Phase 0, parse it for endpoint inventory.

**Output:** `system-model.md` Section 1.2. Begin populating
`record-type-catalog.json` with the endpoint/resource inventory.

**Include deprecation status from Phase 0** — document if the chosen API is
modern/stable vs. deprecated. This is the single most expensive research
mistake to discover late.

### Phase 3: Data model (Section 1.3)

For `candidate` scope: high-level entity inventory with relationships;
field-level detail not required.

For `build-ready` scope: complete field-level detail for the planned roadmap
phase's record types.

**Must answer:**
- All record/resource types
- Fields, types, constraints, required/optional
- Relationships between types
- State machines for stateful records (e.g., `invoice: draft → open → paid → voided`)
- Identification scheme (IDs, external IDs, natural keys)
- Query/search/pagination capabilities

**Method:** Schema extraction (if OpenAPI available), SDK deep audit (read SDK
source), doc tree crawl.

**Output:** `record-type-catalog.json`:

```json
{
  "platform": "{platform}",
  "api_version": "{version}",
  "cataloged_at": "{ISO 8601}",
  "types": [
    {
      "name": "{TypeName}",
      "api_path": "/api/v1/{types}",
      "operations": ["create", "read", "update", "delete", "list"],
      "fields": [
        {
          "name": "{field}",
          "type": "string|int|bool|object|array",
          "required": true,
          "description": "",
          "constraints": {}
        }
      ],
      "relationships": [
        {"type": "belongs_to", "target": "{OtherType}", "field": "{foreign_key}"}
      ],
      "state_machine": {
        "states": ["draft", "active", "archived"],
        "transitions": [
          {"from": "draft", "to": "active", "trigger": "activate"}
        ]
      },
      "pagination": {"type": "cursor|page|offset", "default_page_size": 100}
    }
  ]
}
```

**Note:** for `candidate` scope, restrict the catalog to the planned roadmap
phase's types — don't try to catalog 100+ types when only 10 are in scope.

### Phase 4: Authentication and protection (Section 1.4)

**Must answer:**
- Authentication mechanisms per API surface
- Authorization/permission model (scopes, roles)
- Rate limiting and throttling (limits, headers, backoff)
- Request/response size limits

**This must be answered completely even for `candidate` scope.** Auth must
work for any phase of twin implementation; surfacing an auth blocker late is
the most expensive feasibility miss.

**Output:** `system-model.md` Section 1.4. Document the twin's auth simulation
strategy: full OAuth flow vs. simplified bearer-token acceptance vs. accept-any-
header.

### Phase 5: Event model (Section 1.5)

**Must answer:**
- Does the platform provide native webhooks?
- What events exist, what triggers them, what is the payload?
- Delivery semantics (at-least-once, retry, ordering)
- Subscription/registration API
- Polling patterns when events aren't available

**Output:** `event-model.md`:

```markdown
# Event Model: {Platform}

## Classification
- [ ] Native webhooks with registration API
- [ ] Native webhooks, manual configuration only
- [ ] User-constructed events (scripting/triggers)
- [ ] Polling only (no native events)

## Events
| Event | Trigger | Payload Shape | Delivery |
|-------|---------|---------------|----------|
| ... | ... | ... | ... |

## Twin Strategy
{How the twin will implement events — webhook signing scheme, event types, dispatcher configuration}
```

Many platforms lack native webhooks (e.g., NetSuite). Document this explicitly
— "polling only" is a finding, not an absence of finding.

### Phase 6: Customization and extensibility (Section 1.6)

**Must answer:**
- Can customers add custom fields?
- Can customers write custom code (e.g., SuiteScript, Apex)?
- Are there workflow engines (e.g., NetSuite SuiteFlow)?
- Can the twin simulate generic customizations?
- What is out of scope (customer-specific logic)?

**Principle:** the twin simulates platform APIs, not customer customizations.
Customer-specific code is permanently out of scope.

**Output:** `system-model.md` Section 1.6.

### Phase 7: Source quality (Section 1.7 wrap-up)

**Must answer:**
- How good is the official documentation?
- Are there mature SDKs in target languages?
- Are there code examples, Postman collections?
- What are the gaps in documentation?
- Where will we get answers during build?

**Output:** `system-model.md` Section 1.7. Update `source-catalog.json` with
quality ratings.

### Phase 8: Release lifecycle (Section 1.8)

**Must answer:**
- Versioning model (URL, header, date-based)
- Release cadence (continuous, quarterly, annual)
- Deprecation policy and timeline
- Breaking change history
- What APIs are deprecated or being sunset (from Phase 0 findings)

**Output:** `system-model.md` Section 1.8. Write `release-timeline.json`:

```json
{
  "platform": "{platform}",
  "release_cadence": "continuous|quarterly|biannual|annual",
  "versioning_model": "{description}",
  "deprecation_policy": "{description}",
  "current_version": "{version}",
  "versions": [
    {
      "version": "{version}",
      "release_date": "{date}",
      "eol_date": "{date|null}",
      "breaking_changes": [],
      "notes": ""
    }
  ]
}
```

### Phase 8.5: SDK behavioral audit

**Goal:** Discover behavioral quirks by auditing SDK source code and community
knowledge.

**Must answer:**
- What workarounds do SDKs implement?
- What issues do developers report (GitHub issues, Stack Overflow)?
- What validation rules, edge cases, error handling patterns exist?
- What are the "gotchas" (null vs. empty string, timezone handling, etc.)?

**Output:** `quirk-hypotheses.json` — 15-30 behavioral quirks to verify and
implement. Organize by domain:

- AUTH (auth scheme deviations, token format quirks)
- IDENTITY (user/account ID generation, external IDs)
- CRUD (validation rules, default values, partial updates)
- QUERY (filter syntax, search behavior, includes)
- LIFECYCLE (state machine edge cases, soft delete behavior)
- ERROR_HANDLING (status code conventions, error envelope shape)
- PAGINATION (cursor format, has_more semantics, default page sizes)
- TIMESTAMPS (timezone handling, ISO format variations, epoch precision)
- NULL_HANDLING (null vs. omitted vs. empty string)

Focus on high-priority domains (AUTH, IDENTITY, CRUD) first. Lower-priority
domains can be deferred.

### Phase 9: Synthesis and decision

**1. Write `feasibility.md`:**

Cover:
- Is there enough public information to build a useful twin?
- What is the SDK compatibility strategy?
- What is the estimated scope (number of endpoints, complexity)?
- Go/no-go recommendation with rationale

**2. Write `architecture-spec.md`:**

For approved candidates, produce the technical design:

- **Engine analysis** (mandatory section): which `twinkit/` engines (workspace,
  ledger, messaging, events, search, or none) the twin will use. Whether the
  existing engine satisfies requirements or has gaps. If gaps: enhancement of
  existing engine, or new engine required. **"New engine required" blocks
  generation** — the engine must land first as a separate work stream.
- Directory structure (per `wondertwin/CLAUDE.md` "Standard structure")
- API surface (endpoints to implement)
- Auth simulation strategy (per Phase 4)
- Key design decisions
- What is in scope vs. out of scope for v0.1

**3. Write `compatibility-path.md`:**

```markdown
# Compatibility Path: {Platform}

## SDK Spectrum Position
{Position from: multiple canonical, single canonical, canonical + community, community only, API docs + schema, API docs only, restricted access}

## Recommended Compatibility Target
- **Primary**: {SDK package}@{version} ({language})
- **Secondary**: {if applicable}
- **Conformance gate**: {What tests/checks confirm compatibility}

## Risk Factors
- {SDK staleness, undocumented behaviors, breaking change frequency, etc.}

## Build Strategy
{How to build the twin given the SDK landscape — test against SDK tests, build from OpenAPI, etc.}
```

**4. Write `roadmap.json`:**

```json
{
  "platform": "{platform}",
  "phases": [
    {
      "id": "phase-1",
      "name": "{description}",
      "scope": "{what is included}",
      "record_types": ["{types covered}"],
      "endpoints": ["{endpoints}"],
      "depends_on": [],
      "estimated_files": 0
    }
  ]
}
```

**5. Update `RESEARCH-STATUS.md`** with final confidence scores and the
remaining gap list.

### Phase 10: Version pinning and provenance

Before handing off to the twin generator, lock down the provenance record that
pins the twin to specific versions.

**1. Append the completion entry to `provenance-log.jsonl`:**

```json
{"timestamp": "{ISO 8601}", "action": "research_complete", "scope": "{scope}", "api_version": "{pinned version}", "sdk_version": "{pinned version}", "archive_sha": "{git commit of archive}"}
```

**2. Generate the twin's `provenance.json` template:**

This is the file that ships with the twin (in `twin-{name}/provenance.json`).
It validates against `wondertwin/schemas/provenance.schema.json`:

```json
{
  "twin": "twin-{name}",
  "version": "0.1.0",
  "api_version": "{pinned API version}",
  "platform": "{Platform}",
  "platform_url": "{developer docs URL}",
  "category": "{category}",
  "research_archive": "wondertwin-docs/research/{platform}",
  "research_completed": "{ISO 8601}",
  "generated_at": "{ISO 8601 timestamp}",
  "sources": [
    {
      "type": "{source_type}",
      "url": "{url}",
      "accessed": "{date}",
      "archived_at": "{archive_path}"
    }
  ],
  "twinkit_packages": [],
  "sdk_target": {
    "package": "{sdk_import_path}",
    "version": "{pinned version}",
    "language": "go"
  },
  "scope": {
    "implemented": [],
    "not_implemented": []
  }
}
```

## RESEARCH-STATUS template

`RESEARCH-STATUS.md` is the live document the build gate consults. It tracks
confidence per Section-1 area and the **gap list with expected-value**.

```markdown
# Research Status: {Platform}

**Category**: {category}
**Scope**: {scope}
**Initiated**: {date}
**Status**: In Progress | Build-Ready | Complete

## Confidence Scores

| Area | Confidence | Notes |
|------|-----------|-------|
| 1.1 System identity            | 0.0 | Not started |
| 1.2 API surface                | 0.0 | Not started |
| 1.3 Data model                 | 0.0 | Not started |
| 1.4 Auth & protection          | 0.0 | Not started |
| 1.5 Event model                | 0.0 | Not started |
| 1.6 Customization              | 0.0 | Not started |
| 1.7 Source quality             | 0.0 | Not started |
| 1.8 Release lifecycle          | 0.0 | Not started |

## Gap List (sorted by expected value of filling)

Each gap entry includes:
- **What artifact closes the gap** — concrete deliverable
- **Confidence improvement** — what score moves and by how much
- **Residual twin-functionality risk if NOT filled** — concrete failure mode

| # | Gap | Closing artifact | Confidence delta | Risk if unfilled |
|---|-----|------------------|------------------|------------------|
| 1 | ... | ... | ... | ... |

## Open Questions

(populated as research progresses)

## Next Actions

(populated as research progresses; default to highest-EV gap as next target)
```

The gap list drives prioritization. When the build-skill consults this file at
its Phase 0, **high-EV gaps unfilled** = research not yet build-ready, return
to research with the highest-EV gap as the next target.

## Confidence scoring guide

| Score | Meaning |
|-------|---------|
| 0.0–0.2 | No information or only vague references |
| 0.3–0.4 | Partial information from low-reliability sources |
| 0.5–0.6 | Reasonable coverage from mixed sources, some gaps |
| 0.7–0.8 | Good coverage from reliable sources, minor gaps |
| 0.9–1.0 | Comprehensive coverage from authoritative sources, corroborated |

## Provenance rules

Every claim in research output must link to its source.

1. **Every claim cites sources.** Use inline citations: `[source: {source_id}]`
2. **Single-source claims carry the source's confidence level.** Official docs = high; community = medium; undated forum posts = low.
3. **Multi-source agreement elevates confidence.** Two independent sources agreeing = higher than either alone.
4. **Community-only claims are flagged.** Mark as "community-reported, unverified" until confirmed against official sources.
5. **Contradictions are recorded, not hidden.** When sources disagree, record both and flag for resolution.
6. **Provenance is append-only.** When a claim is updated, the old record is superseded, not deleted.
7. **Dates matter.** Always record when a source was accessed. A 2024 doc page accessed in 2026 may describe outdated behavior.

## Behavior observation (respect ToS)

Some research requires running the SDK or hitting the platform's sandbox to
discover behavior that docs don't describe. Rules:

- **Use sandbox / test environments only.** Never run probes against production tenants you don't own.
- **Read the platform's Terms of Service.** Some platforms prohibit reverse-engineering or automated probing even of sandbox endpoints. If ToS prohibits, document the gap and rely on doc + SDK sources only.
- **Archive observations with provenance.** Record the request shape, response shape, timestamps, and the credentials class used (sandbox / test account). Never archive real customer credentials.
- **Disclose probe-discovered behavior in commit messages.** When a quirk is implemented based on probing rather than docs, the commit cites the observation file.

For platforms whose ToS prohibits even sandbox probing (rare), the twin is
limited to doc-based behavior. Note this limitation in `feasibility.md`.

## SPA Documentation Extraction Playbook

Many modern API docs (Readme.io, Redocly, Docusaurus, Stoplight) are SPAs that
return empty shells to plain HTTP fetch. Use a headless browser to extract
rendered content.

### Readme.io (proven technique)

Readme.io sites embed OpenAPI specs in page metadata. This is the most
valuable extraction target — a single spec file contains every endpoint,
schema, query parameter, and enum.

**Step 1:** Fetch any `/reference/` page; the HTML contains a JSON registry
of all spec files with UUIDs and filenames.

**Step 2:** Extract the spec registry by parsing embedded JSON in the page's
`<script type="application/json">` blocks. Registry entries contain:
`filename`, `uuid`, `type` (`"openapi"`), `last_updated`.

**Step 3:** Fetch the spec directly via a URL like
`https://docs.{platform}.com/openapi/{uuid}` — returns the full OpenAPI
YAML/JSON.

**Key patterns:**
- Spec files typically named `{product}-openapi.yaml` or `{product}-api.json`
- Multiple specs may exist (main API, OAuth, onboarding, etc.)
- UUIDs are stable — can be re-fetched in future research passes
- Spec `last_updated` timestamps indicate freshness

**Fallback if direct URL 404s:** parse the embedded JSON registry from the
SPA's hydration data and build the URL manually.

### Other SPA platforms

| Platform | Extraction strategy | Status |
|---|---|---|
| Readme.io | OpenAPI spec from page metadata (above) | **Proven** (Mercury) |
| Redocly | Look for `__redoc_state` or bundled OpenAPI in `<script>` tags | Untested |
| Docusaurus | Content often in static JSON; check `/api/` routes | Untested |
| Stoplight | OpenAPI typically at predictable paths (`/api.yaml`) | Untested |
| Custom React | Headless fetch with `wait_for` selector, then parse HTML | Case-by-case |

### General SPA tips

- Always try a headless fetch with `wait_for` targeting content selectors (`article`, `main`, `[class*="content"]`)
- Extract structured data (sidebar links, endpoint lists, schema tables) as JSON via in-page `evaluate` calls
- Use action-driven fetches to expand collapsed sections or navigate paginated docs
- Archive raw extractions in `archive/` with provenance notes

## Common mistakes

### Research mistakes
- **Assuming docs are complete.** Documentation routinely omits edge cases, error shapes, behavioral quirks. Treat docs as a starting point, not ground truth.
- **Ignoring community sources.** Forum posts and GitHub issues often contain the most valuable behavioral observations.
- **Not archiving.** URLs go stale. Doc pages change. Always fetch and archive locally before citing.
- **Conflating versions.** A blog post from 2023 may describe behavior that changed in 2025. Always check recency.

### Provenance mistakes
- **Blanket confidence scores.** Never assign a single confidence score to an entire platform. Score per Section-1 subsection.
- **Forgetting to pin versions.** The twin must be pinned to a specific API version and SDK version. "Latest" is not a version.
- **Not recording access dates.** A URL accessed on 2026-03-17 is a different source than the same URL accessed on 2026-06-01 if the content changed.

### Scope mistakes
- **Researching everything for a candidate assessment.** `candidate` scope only needs enough for go/no-go. Don't do field-level data modeling until approved.
- **Skipping auth for candidate scope.** Auth must be understood even for candidates — exotic or undocumented auth is a feasibility risk.
- **Not catching deprecations early.** Phase 0's deprecation searches are not optional. Building on a sunset API wastes the entire effort.

### Engine-analysis mistakes
- **Treating engine selection as code-generation-time concern.** Engine choice is a research-stage decision recorded in `architecture-spec.md`. The build gate verifies engine analysis is complete; "new engine required" blocks generation.
- **Forcing a fit.** If no existing `twinkit/` engine fits, document the gap honestly. Cramming an entity model into the wrong engine produces twins that drift from the canonical pattern.

## Learning capture

Every research pass produces two kinds of output:

1. **Platform-specific artifacts** (system-model, feasibility, etc.)
2. **Generalizable techniques, patterns, and discoveries** that improve the research process itself

The second kind is easily lost if not explicitly captured. At the end of every
research pass, before declaring it complete, ask:

1. **New extraction technique?** A way to get data from a previously inaccessible source? (e.g., Readme.io OpenAPI extraction.) → add to "SPA Documentation Extraction Playbook" above.
2. **New source pattern?** A class of source that applies across platforms? → add to Phase 0 activities.
3. **Process improvement?** A phase ordering, tool combination, or strategy that worked better than this skill describes? → update the relevant phase.
4. **Correction to assumptions?** A common assumption that turned out wrong? (e.g., "Bearer auth" turning out to be Basic auth.) → reinforce in the relevant phase.
5. **Tool capability gap?** A wall hit that a new tool or technique could solve for future passes? → flag in the platform's `archive/` and surface upstream.

**Capture rules:**
- Be specific — "Readme.io embeds OpenAPI specs in page metadata with UUID-based URLs" beats "SPA docs can be scraped"
- Include provenance — which platform research produced this learning? What date?
- Mark confidence — Proven (tested on a real platform) vs. Untested (hypothesis from observation)
- Update the skill file directly — learnings deferred are learnings lost

## Integration with the build skill

This skill produces the artifact set the community twin generator
(`wondertwin/skills/community-twin-generator.md`) consumes at its Phase 0.
The build gate is documented in
`wondertwin-docs/skills/handoffs/research-to-build.md` (which lives in the
docs repo so both community and pro generators reference the same gate).

Research is iterative. Findings update with each pass. The gap list shrinks.
Build-readiness emerges over multiple research passes, not in one shot. The
build gate is "met the floor for this twin's planned coverage", not "research
is final."
