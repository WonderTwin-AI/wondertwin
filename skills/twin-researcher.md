---
skill: twin-researcher
skill_version: "1.0"
schemas:
  provenance.schema.json: "1.1"
---

# SKILL: WonderTwin Twin Researcher

## Purpose

Investigate a target platform well enough to build a twin against it, and produce archived, provenanced research artifacts that pin the twin to a specific version of the platform's API. This skill is the entry point in the twin lifecycle — it precedes the twin generator and feeds into the twin maintainer.

Research starts from inquiry, not confirmation. The goal is to build a mental model of the target system complete enough to simulate its observable behavior — and to know where our model is uncertain.

This skill produces two categories of output:
1. **Research artifacts** — structured knowledge about the target system, archived with provenance
2. **Decision artifacts** — feasibility assessment, architecture spec, and build roadmap

## When to Use

- Evaluating a new platform as a twin candidate
- Deepening knowledge before building a new twin (feeds into `twin-generator`)
- Investigating a specific platform area for a twin extension (feeds into `twin-extender`)
- Responding to a platform release that may affect an existing twin (feeds into `twin-maintainer`)

## Prerequisites

1. A target platform name and general category (accounting, payments, CRM, etc.)
2. Access to public internet for documentation, SDK repos, and community sources
3. The `wondertwin-docs/` repo for storing research archives

## Agent Permissions

When this skill runs as a subagent, it needs write access to `wondertwin-docs/research/` which is **outside** the main `wondertwin/` working directory. To avoid permission denials:

- The **parent conversation** should pre-create the research directory structure before launching the agent:
  ```bash
  platform="{platform}"
  base="/Users/tela/dev/wondertwin-docs/research/${platform}"
  mkdir -p "${base}/archive/"{official-docs,schemas,sdks,release-notes,community,observations}
  ```
- Alternatively, the parent should write all artifact files itself after the agent completes research, since the parent conversation has broader file permissions than subagents.
- Subagents spawned from the `wondertwin/` directory cannot write to `wondertwin-docs/` due to sandbox scoping. This is a known limitation.

## Inputs

The user will provide:

- **Platform name**: The SaaS platform to research (e.g., "QuickBooks Online", "Plaid", "Modern Treasury")
- **Category**: The platform's functional category (e.g., "accounting", "banking-as-a-service", "money-movement")
- **Research scope**: One of:
  - `candidate` — Minimum viable research for go/no-go (Sections 1.1, 1.2, 1.4, 1.7, 1.8)
  - `build-ready` — Full research for first implementation phase (all Section 1 questions for phase 1 scope)
  - `targeted` — Investigate a specific area (user specifies which Section 1 questions)
- **Known context** (optional): Anything the user already knows — SDK names, API docs URLs, prior research
- **Fintech primitives** (optional, for fintech twins): Which `twinkit/` engines this platform is expected to use

## Output Structure

All research artifacts are stored under `wondertwin-docs/research/{platform}/`:

```
wondertwin-docs/research/{platform}/
├── system-model.md              # Answers to Section 1 questions with confidence scores
├── record-type-catalog.json     # Inventory of all types, fields, operations, relationships
├── compatibility-path.md        # SDK landscape, spectrum position, build strategy
├── event-model.md               # Webhook/event classification and twin strategy
├── release-timeline.json        # Historical and projected releases, deprecation dates
├── observations.json            # Notable API behaviors with provenance
├── feature-gap-register.json    # What the twin does NOT cover, by domain
├── source-catalog.json          # Index of all discovered and archived sources
├── provenance-log.jsonl         # Append-only log of research activities
├── feasibility.md               # Go/no-go with rationale
├── architecture-spec.md         # Technical design for twin implementation
├── roadmap.json                 # Phased build plan
├── archive/                     # Archived source materials
│   ├── official-docs/           # Fetched doc tree, versioned by date
│   │   ├── {date}/
│   │   └── latest -> {date}/
│   ├── schemas/                 # OpenAPI specs, WSDL files, metadata
│   │   ├── {api-version}/
│   │   └── latest -> {api-version}/
│   ├── sdks/                    # SDK metadata and analysis (not full clones)
│   │   └── {sdk-name}@{version}.md
│   ├── release-notes/           # Archived release notes per version
│   │   └── {version}.md
│   ├── community/               # Forum threads, blog posts, SO answers
│   │   └── {source-id}.md
│   └── observations/            # Behavioral observations from probing
│       └── {date}-{description}.md
└── RESEARCH-STATUS.md           # Current state, confidence/curiosity scores, next steps
```

---

## Process

### Phase 0: Initialize Research Directory

Create the research directory structure and initialize tracking files.

**1. Create the directory tree:**

```bash
platform="{platform}"  # lowercase, hyphenated (e.g., "quickbooks-online")
base="wondertwin-docs/research/${platform}"
mkdir -p "${base}/archive/"{official-docs,schemas,sdks,release-notes,community,observations}
```

**2. Initialize `source-catalog.json`:**

```json
{
  "platform": "{platform}",
  "display_name": "{Platform Display Name}",
  "category": "{category}",
  "research_initiated": "{ISO 8601}",
  "sources": []
}
```

**3. Initialize `provenance-log.jsonl`:**

Write the first entry:

```json
{"timestamp": "{ISO 8601}", "action": "research_initiated", "scope": "{candidate|build-ready|targeted}", "agent": "twin-researcher"}
```

**4. Initialize `RESEARCH-STATUS.md`:**

```markdown
# Research Status: {Platform}

**Category**: {category}
**Scope**: {scope}
**Initiated**: {date}
**Status**: In Progress

## Confidence / Curiosity Scores

| Area | Confidence | Curiosity | Notes |
|------|-----------|-----------|-------|
| 1.1 What is this system? | 0.0 | — | Not started |
| 1.2 How does it expose itself? | 0.0 | — | Not started |
| 1.3 What does it remember? | 0.0 | — | Not started |
| 1.4 How does it protect itself? | 0.0 | — | Not started |
| 1.5 How does it communicate changes? | 0.0 | — | Not started |
| 1.6 How does it allow customization? | 0.0 | — | Not started |
| 1.7 How can we learn about it? | 0.0 | — | Not started |
| 1.8 How does it change over time? | 0.0 | — | Not started |

## Open Questions

(populated as research progresses)

## Next Actions

(populated as research progresses)
```

---

### Phase 1: Meta-Research (Section 1.7)

Start here regardless of scope. Understanding what sources exist determines how to investigate everything else.

**1. Discover official documentation:**

- Search for the platform's developer documentation portal
- Identify the documentation structure (REST reference, guides, tutorials, changelog)
- Record the base URL and structure in `source-catalog.json`

**2. Discover API specifications:**

- Search for OpenAPI/Swagger specs: check `{api-domain}/openapi.json`, GitHub repos, doc pages
- Search for WSDL, GraphQL schema, or other machine-readable definitions
- If found, fetch and archive to `archive/schemas/{api-version}/`

**3. Survey the SDK landscape:**

- Search major package registries for official and community SDKs:
  - Go: `pkg.go.dev` search
  - Python: PyPI search
  - Node: npmjs.com search
  - Ruby: rubygems.org search
- For each SDK found, record: name, maintainer (official vs. community), version, last updated, stars/downloads
- Assess the SDK spectrum position (see Section 5 of the research framework)

**4. Identify community sources:**

- Search for the platform's developer forum, Slack/Discord, Stack Overflow tag
- Note any iPaaS connector documentation (Workato, Celigo, Tray.io)
- Search GitHub for existing mocks/simulators of this platform (prior art)

**5. Archive and catalog:**

For every source discovered:

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

Append to `source-catalog.json` sources array.

**6. Update confidence scores for 1.7.**

---

### Phase 2: System Identity (Section 1.1)

**Must answer:**
- What does this system do? What problem does it solve?
- What is the conceptual data model? What entities does it think in?
- What are the system's boundaries?
- Who uses it? (End users, developers, integrators)

**Sources to consult:** Official "what is" / overview pages, Wikipedia, vendor marketing, industry analysis.

**For fintech twins:** Identify which of the seven fintech primitives (money movement, ledger, card, identity, risk, billing, compliance) this platform maps to. This determines which `twinkit/` engines to use.

**Output:** Write the 1.1 section of `system-model.md` with provenance citations.

---

### Phase 3: API Surface Discovery (Section 1.2)

**Must answer:**
- What API protocols exist? (REST, SOAP, GraphQL, gRPC, webhooks)
- For each: status (active, deprecated, emerging)
- Do protocols share underlying state?
- URL structure, versioning model, serialization formats

**Method:** Doc tree crawl of API reference. If OpenAPI spec was found in Phase 1, parse it for endpoint inventory.

**Output:** Write the 1.2 section of `system-model.md`. Begin populating `record-type-catalog.json` with the endpoint/resource inventory.

---

### Phase 4: Auth & Protection (Section 1.4)

**Must answer:**
- Authentication mechanisms per API surface
- Authorization/permission model
- Rate limiting and throttling (limits, headers, backoff)
- Request/response size limits

**This must be answered completely even for `candidate` scope** — auth must work for any phase of twin implementation.

**Output:** Write the 1.4 section of `system-model.md`.

---

### Phase 5: Data Model Deep Dive (Section 1.3)

**For `candidate` scope:** High-level entity inventory with relationships. Field-level detail is not required.

**For `build-ready` scope:** Complete field-level detail for phase 1 record types.

**Must answer:**
- All record/resource types
- Fields, types, constraints, required/optional
- Relationships between types
- State machines for stateful records
- Identification scheme (IDs, external IDs, natural keys)
- Query/search/pagination capabilities

**Method:** Schema extraction (if OpenAPI available), SDK deep audit (read source of target SDK), doc tree crawl.

**Output:** Complete `record-type-catalog.json`:

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

---

### Phase 6: Event Model (Section 1.5)

**Must answer:**
- Does the platform provide native webhooks?
- What events exist, what triggers them, what is the payload?
- Delivery semantics (at-least-once, retry, ordering)
- Subscription/registration API
- Polling patterns when events aren't available

**Output:** Write `event-model.md`:

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
{How the twin will implement events — which webhook package features to use, signing scheme, etc.}
```

---

### Phase 7: Release Lifecycle (Section 1.8)

**Must answer:**
- Release cadence and naming
- Versioning model (URL, header, date-based)
- Deprecation policy and timeline
- Breaking change history
- Interface velocity vs. logic velocity vs. platform velocity

**Output:** Write `release-timeline.json`:

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

---

### Phase 8: Compatibility Path Assessment (Section 5)

Using findings from Phases 1-7, assess the SDK compatibility path.

**Output:** Write `compatibility-path.md`:

```markdown
# Compatibility Path: {Platform}

## SDK Spectrum Position
{Position from: multiple canonical, single canonical, canonical + community, community only, API docs + schema, API docs only, restricted access}

## Recommended Compatibility Target
- **Primary**: {SDK package}@{version} ({language})
- **Secondary**: {if applicable}
- **Conformance gate**: {What tests/checks confirm compatibility}

## Risk Factors
- {List risks: SDK staleness, undocumented behaviors, breaking change frequency, etc.}

## Build Strategy
{How to build the twin given the SDK landscape — test against SDK tests, build from OpenAPI, etc.}
```

---

### Phase 9: Synthesis & Decision

**1. Write `feasibility.md`:**

Assess whether a twin is viable. Cover:
- Is there enough public information to build a useful twin?
- What is the SDK compatibility strategy?
- What is the estimated scope (number of endpoints, complexity)?
- What is the commercial demand signal?
- Go/no-go recommendation with rationale

**2. Write `architecture-spec.md`:**

For approved candidates, produce the technical design:
- Which `twinkit/` packages to use (and which new ones to build)
- For fintech twins: primitive composition and hook requirements
- Directory structure
- API surface (endpoints to implement)
- Key design decisions
- What is in scope vs. out of scope for v0.1

**3. Write `roadmap.json`:**

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

**4. Update `RESEARCH-STATUS.md`** with final confidence/curiosity scores and status.

---

### Phase 10: Version Pinning & Provenance

Before handing off to the twin generator, lock down the provenance record that pins the twin to specific versions.

**1. Finalize `provenance-log.jsonl`:**

Append the completion entry:

```json
{"timestamp": "{ISO 8601}", "action": "research_complete", "scope": "{scope}", "api_version": "{pinned version}", "sdk_version": "{pinned version}", "archive_sha": "{git commit of archive}"}
```

**2. Generate the twin's `provenance.json` template:**

This is the file that ships with the twin (in `twin-{name}/provenance.json`). It extends the existing provenance format with research lineage:

<!-- schema: provenance.schema.json -->
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
  "fintech_primitives": [],
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

---

## Provenance Rules

Every claim in research output must link to its source. Follow these rules:

1. **Every claim cites sources.** Use inline citations in markdown: `[source: {source_id}]`
2. **Single-source claims carry the source's confidence level.** Official docs = high, community = medium, undated forum posts = low.
3. **Multi-source agreement elevates confidence.** Two independent sources agreeing = higher than either alone.
4. **Community-only claims are flagged.** Mark as "community-reported, unverified" until confirmed against official sources.
5. **Contradictions are recorded, not hidden.** When sources disagree, record both and flag for resolution.
6. **Provenance is append-only.** When a claim is updated, the old record is superseded, not deleted.
7. **Dates matter.** Always record when a source was accessed. A 2024 doc page accessed in 2026 may describe outdated behavior.

## Confidence Scoring Guide

| Score | Meaning |
|-------|---------|
| 0.0–0.2 | No information or only vague references |
| 0.3–0.4 | Partial information from low-reliability sources |
| 0.5–0.6 | Reasonable coverage from mixed sources, some gaps |
| 0.7–0.8 | Good coverage from reliable sources, minor gaps |
| 0.9–1.0 | Comprehensive coverage from authoritative sources, corroborated |

## Curiosity Scoring Guide

| Score | Meaning |
|-------|---------|
| Low | Area is well-understood or not important for twin fidelity |
| Medium | Some expected value from additional research |
| High | Significant undiscovered knowledge likely — behavioral quirks, undocumented features, or commercial intelligence |

---

## Common Mistakes

### Research mistakes
- **Assuming docs are complete.** Official documentation routinely omits edge cases, error shapes, and behavioral quirks. Treat docs as a starting point, not ground truth.
- **Ignoring community sources.** Forum posts and GitHub issues often contain the most valuable behavioral observations — the things docs don't say.
- **Not archiving.** URLs go stale. Doc pages change. Always fetch and archive locally before citing.
- **Conflating versions.** A blog post from 2023 may describe behavior that changed in 2025. Always check recency.

### Provenance mistakes
- **Blanket confidence scores.** Never assign a single confidence score to an entire platform. Score per knowledge area (Section 1 subsection).
- **Forgetting to pin versions.** The twin must be pinned to a specific API version and SDK version. "Latest" is not a version.
- **Not recording access dates.** A URL accessed on 2026-03-17 is a different source than the same URL accessed on 2026-06-01 if the content changed.

### Scope mistakes
- **Researching everything for a candidate assessment.** `candidate` scope only needs enough for go/no-go. Don't do field-level data modeling until approved.
- **Skipping auth for candidate scope.** Auth must be understood even for candidates — if auth is exotic or undocumented, that's a feasibility risk.
- **Not identifying the fintech primitive composition early.** For fintech twins, the primitive mapping determines which `twinkit/` engines to use and must be established in Phase 2.

---

## Integration with Other Skills

| Skill | Handoff |
|-------|---------|
| `twin-generator` | Research produces `architecture-spec.md`, `provenance.json` template, and `roadmap.json`. Generator consumes these as inputs. |
| `twin-extender` | Targeted research on a new area produces updated `record-type-catalog.json` and `feature-gap-register.json`. Extender uses these to add capabilities. |
| `twin-maintainer` | Research triggered by platform release produces delta analysis. Maintainer uses this to plan updates. |

## Archive Maintenance

- Research archives live in `wondertwin-docs/research/{platform}/`
- Archives are committed to the `wondertwin-docs` repo with descriptive commit messages
- The `source-catalog.json` is the index — it must be kept in sync with archived files
- When a source is re-fetched (e.g., docs updated after a platform release), archive the new version with a new date directory. Do not overwrite previous versions.
