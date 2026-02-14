# WonderTwin Infrastructure Plan

> Standard build, release, and distribution infrastructure for the WonderTwin ecosystem.
> 
> **Release tooling: [GoReleaser](https://goreleaser.com/) (OSS edition)**

---

## Ecosystem Architecture

WonderTwin is a multi-repo ecosystem. Every twin lives in its own repo. This is a deliberate design decision that supports:

- Commercial users who maintain private twins in their own repos
- Independent versioning — a twin updates when its maintainer is ready
- Contributor isolation — PRs to one twin don't touch another
- Scale to thousands of twin repos without monorepo pain

### Repos

| Repo | Purpose | Release artifact |
|---|---|---|
| `wondertwin` | CLI (`wt`) | `wt` binaries |
| `twinkit` | Shared Go libraries for building twins | Go module |
| `twin-{name}` | Individual behavioral API twin | Twin binary |
| `wondertwin-site` | Marketing site (wondertwin.ai) | Static files → S3/CloudFront |
| `homebrew-tap` | Homebrew formulae for `wt` CLI | Formula files |

All repos live under the `wondertwin-ai` GitHub org.

---

## Dependency Model

```
github.com/wondertwin-ai/twinkit  ← Go module, tagged (v0.1.0+)
    ↑
    | go.mod dependency (pinned version)
    |
twin-stripe/
twin-twilio/
twin-resend/
twin-clerk/
twin-posthog/
twin-logodev/
... (thousands more)
```

**No automatic cascading.** When `twinkit` releases a new version, twin repos are NOT automatically rebuilt. Twin maintainers bump their `go.mod` dependency when they're ready. This is how the real Go ecosystem works, and it's how commercial users expect to operate — nobody wants upstream CI triggering builds in their private repo.

For first-party twins maintained by WonderTwin, Dependabot (or similar) can open PRs to nudge `twinkit` version bumps. These are reviewed and merged on the twin's own schedule.

---

## Platform Targets

Every binary (CLI and twins) is cross-compiled for:

| OS | Arch | Notes |
|---|---|---|
| linux | amd64 | CI runners, cloud servers |
| linux | arm64 | ARM servers, Graviton |
| darwin | amd64 | Intel Macs |
| darwin | arm64 | Apple Silicon |

Windows is excluded. Revisit if demand appears.

Binary naming convention (managed by GoReleaser):
- CLI: `wt_{{ .Version }}_{{ .Os }}_{{ .Arch }}`
- Twins: `twin-{name}_{{ .Version }}_{{ .Os }}_{{ .Arch }}`

GoReleaser generates tarballs per platform plus a checksums file automatically.

---

## GoReleaser

[GoReleaser](https://goreleaser.com/) is the standard release tool across the entire WonderTwin ecosystem. It replaces custom cross-compilation scripts, checksum generation, GitHub Release creation, artifact upload, and Homebrew formula management with a single declarative config.

**Edition:** OSS (free). Pro features (monorepo support, custom publishers, Docker manifests) are not needed for the multi-repo architecture.

**Current state:** A contributor has a GoReleaser PR in progress for the `wondertwin` CLI repo. The existing `release.yml` is a hand-rolled matrix cross-compilation workflow that will be replaced when this PR lands.

### Why GoReleaser

- Single config file (`.goreleaser.yml`) handles the entire release pipeline
- Native Go cross-compilation — no matrix builds, no target OS runners
- Built-in GitHub Releases integration — creates release, uploads artifacts, generates changelog
- Built-in Homebrew tap support — auto-generates and pushes formula on CLI release
- Built-in checksum generation and optional signing
- Snapshot mode for testing releases without tagging
- Widely adopted in the Go ecosystem — contributors will recognize it immediately

### Standard Twin `.goreleaser.yml`

```yaml
version: 2

before:
  hooks:
    - go mod tidy
    - go test ./...

builds:
  - main: ./cmd/twin-{{ .ProjectName }}/
    binary: "{{ .ProjectName }}"
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'
```

> **Note:** Registry update is handled separately — see Registry section below.

### CLI `.goreleaser.yml`

Same as twin config, plus Homebrew tap integration:

```yaml
version: 2

before:
  hooks:
    - go mod tidy
    - go test ./...

builds:
  - main: ./cmd/wt/
    binary: wt
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    name_template: "wt_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

brews:
  - repository:
      owner: wondertwin-ai
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    name: wt
    homepage: https://wondertwin.ai
    description: "CLI for WonderTwin — behavioral API twins for local dev and testing"
    install: |
      bin.install "wt"
    test: |
      system "#{bin}/wt", "version"
```

> **Note:** The `HOMEBREW_TAP_TOKEN` is a GitHub PAT with write access to the `homebrew-tap` repo. Set as a repo secret in the `wondertwin` repo's GitHub Actions settings.

---

## Release Process

### Twin Repos

Every twin repo follows an identical release process. The twin generator skill produces repos that are release-ready with zero additional configuration.

**Trigger:** Push a git tag matching `v*` (e.g., `v0.1.0`, `v0.2.3`)

**Pipeline (handled by GoReleaser):**
1. Run `go mod tidy` and `go test ./...`
2. Cross-compile for all 4 platform targets (CGO disabled)
3. Create tarballs per platform
4. Generate SHA256 checksums file
5. Generate changelog from commits since last tag
6. Create GitHub Release from the tag
7. Upload all archives + checksums to the GitHub Release

**Registry update** is a separate step after GoReleaser completes — see Registry section.

**What lives in every twin repo:**
```
twin-{name}/
├── .github/
│   └── workflows/
│       └── release.yml          # GitHub Actions → GoReleaser
├── .goreleaser.yml              # GoReleaser config
├── cmd/twin-{name}/
│   └── main.go
├── internal/
│   ├── api/
│   └── store/
├── Makefile                     # Local dev targets only
├── go.mod
└── go.sum
```

**Makefile targets (local development only — GoReleaser handles releases):**
- `make build` — build for current platform → `./bin/`
- `make test` — run all tests
- `make clean` — remove `bin/` and `dist/`
- `make snapshot` — run `goreleaser release --snapshot --clean` to test release locally without publishing

### CLI Repo (`wondertwin`)

Same GoReleaser process as twin repos, plus:
- GoReleaser auto-generates and pushes Homebrew formula to `homebrew-tap` repo
- Requires `HOMEBREW_TAP_TOKEN` secret in GitHub Actions

### twinkit Releases

`twinkit` is a standalone Go module at `github.com/wondertwin-ai/twinkit`. It's tagged with standard semver (v0.1.0, v0.2.0, etc.).

When `twinkit` is tagged, it's published as a Go module. No binaries, no GoReleaser pipeline — just tests. Twin repos consume it via `go.mod`.

---

## Registry

The registry is the central manifest that maps twin names to versions to download URLs. `wt install stripe@latest` consults the registry to find the right binary.

### Current Implementation

The registry is a **static YAML file** hosted via raw GitHub URL. The `wt` CLI fetches this file to resolve twin names, versions, and download URLs.

### Registry Contents (per twin version)

```yaml
twins:
  stripe:
    latest: "0.2.0"
    versions:
      "0.2.0":
        repo: wondertwin-ai/twin-stripe
        tag: v0.2.0
        binaries:
          linux-amd64: https://github.com/wondertwin-ai/twin-stripe/releases/download/v0.2.0/twin-stripe_0.2.0_linux_amd64.tar.gz
          linux-arm64: https://github.com/wondertwin-ai/twin-stripe/releases/download/v0.2.0/twin-stripe_0.2.0_linux_arm64.tar.gz
          darwin-amd64: https://github.com/wondertwin-ai/twin-stripe/releases/download/v0.2.0/twin-stripe_0.2.0_darwin_amd64.tar.gz
          darwin-arm64: https://github.com/wondertwin-ai/twin-stripe/releases/download/v0.2.0/twin-stripe_0.2.0_darwin_arm64.tar.gz
        checksums_url: https://github.com/wondertwin-ai/twin-stripe/releases/download/v0.2.0/checksums.txt
```

### Registry Update Flow

**Current (manual):** After a twin release, update the YAML file in the registry repo and commit. At 6 twins, this is fine.

**Next step (automated):** Add a `repository_dispatch` step to the twin release workflow that triggers a workflow in the registry repo. The registry workflow receives the twin name, version, and tag, auto-generates the download URLs (which follow a predictable pattern from GitHub Releases), fetches the checksums, and commits the updated YAML.

```yaml
# Added to twin release.yml, after GoReleaser step
- name: Update registry
  uses: peter-evans/repository-dispatch@v3
  with:
    token: ${{ secrets.REGISTRY_UPDATE_TOKEN }}
    repository: wondertwin-ai/wondertwin-registry
    event-type: twin-release
    client-payload: '{"twin": "${{ github.event.repository.name }}", "version": "${{ github.ref_name }}", "repo": "${{ github.repository }}"}'
```

The registry repo has a workflow that listens for `twin-release` events and auto-updates the YAML.

**Future (API):** When the static YAML won't scale (100+ twins with frequent releases), replace with an API-backed registry. The `repository_dispatch` approach migrates cleanly to an API call — same data, different transport.

### Commercial / Private Twins

Commercial users who maintain private twins in their own repos can register their twins with the registry. The same `.goreleaser.yml` template and release workflow work — they just need to configure registry access. The registry accepts updates from any authorized source, not just `wondertwin-ai` org repos.

---

## Distribution Channels

### 1. GitHub Releases (primary)

Every binary is downloadable directly from the repo's GitHub Releases page. GoReleaser creates the release, uploads tarballs and checksums, and generates a changelog. This is the source of truth that all other channels pull from.

### 2. `wt install` (preferred UX)

```bash
wt install stripe@latest
wt install stripe@0.2.0
```

The CLI resolves versions via the registry, downloads the correct platform tarball from GitHub Releases, verifies the checksum, extracts the binary, and places it in the local twin directory.

### 3. Homebrew (CLI only)

```bash
brew install wondertwin-ai/tap/wt
```

GoReleaser auto-updates the `homebrew-tap` repo on CLI release. No manual formula maintenance.

Twins are NOT distributed via Homebrew — there would be thousands of formulae. Twins are installed via `wt install`.

### 4. `go install` (developers)

```bash
go install github.com/wondertwin-ai/wondertwin/cmd/wt@latest
go install github.com/wondertwin-ai/twin-stripe/cmd/twin-stripe@latest
```

Works for anyone with Go installed. Builds from source.

### 5. Install script (convenience)

```bash
curl -sSL https://wondertwin.ai/install.sh | sh
```

Detects OS/arch, downloads the CLI tarball from GitHub Releases, extracts, installs to `/usr/local/bin` or `~/.local/bin`. CLI only — twins are installed via `wt install` after.

---

## CI/CD

### GitHub Actions

All repos use GitHub Actions. Rationale:
- Repos are on GitHub, releases are GitHub Releases, registry updates target GitHub — fighting this grain adds complexity for no benefit
- Free for public repos
- Identical workflow file stamped into every twin repo
- GoReleaser has an official GitHub Action (`goreleaser/goreleaser-action`)
- No infrastructure to maintain

### Workflow: CI (`ci.yml`) — exists today

```yaml
# Trigger: push to main, PRs
# Steps: go vet, go test, go build
```

Lightweight validation on every push and PR. Already in the `wondertwin` repo.

### Workflow: Twin Release (`release.yml`)

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Update registry
        uses: peter-evans/repository-dispatch@v3
        with:
          token: ${{ secrets.REGISTRY_UPDATE_TOKEN }}
          repository: wondertwin-ai/wondertwin-registry
          event-type: twin-release
          client-payload: '{"twin": "${{ github.event.repository.name }}", "version": "${{ github.ref_name }}", "repo": "${{ github.repository }}"}'
```

Single job, ~2 minutes. GoReleaser handles build + release, then registry gets notified.

### Workflow: CLI Release (`release.yml`)

Same structure, with additional env var for Homebrew:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

### No CI for routine twin development

CI runs on tag push only for twin repos. No CI on PRs or branch pushes — the build is a single `go build` that takes seconds locally. Contributors run `make test` before submitting PRs.

The `wondertwin` repo has PR-level CI (`ci.yml`) for the CLI, since changes there affect the user experience.

---

## Hosting

### wondertwin.ai (marketing site)

- S3 static hosting + CloudFront CDN
- ACM certificate (auto-renewing)
- Cloudflare DNS
- Deployed via `deploy.sh` (S3 sync + CloudFront invalidation)

### Registry

- Static YAML file in a GitHub repo, fetched via raw URL
- Scales to ~100 twins without issues
- Migrate to API-backed registry when update frequency or catalog size demands it

---

## Template Files

The following template files are stamped into every new twin repo by the twin generator skill:

```
templates/twin-repo/
├── .github/
│   └── workflows/
│       └── release.yml
├── .goreleaser.yml
├── Makefile
└── .gitignore
```

These are also available in the `wondertwin` repo for reference and for updating existing twin repos.

---

## Secrets Required

### Every twin repo (org-level secrets recommended)

| Secret | Purpose |
|---|---|
| `REGISTRY_UPDATE_TOKEN` | GitHub PAT with repo access to trigger `repository_dispatch` on the registry repo |

`GITHUB_TOKEN` is provided automatically by GitHub Actions.

### CLI repo only (repo-level secret)

| Secret | Purpose |
|---|---|
| `HOMEBREW_TAP_TOKEN` | GitHub PAT with write access to `homebrew-tap` repo |

### Recommendation

Set `REGISTRY_UPDATE_TOKEN` as an **organization-level secret** in the `wondertwin-ai` GitHub org. This way every new twin repo inherits it automatically — no per-repo secret configuration required.

---

## Current State

### What exists today

| Item | Status |
|---|---|
| `wondertwin` repo (CLI) | ✅ On main with CI |
| `twinkit` repo (shared libs) | ✅ Tagged v0.1.0 |
| 6 twin repos (stripe, twilio, resend, posthog, clerk, logodev) | ✅ Separated |
| `wondertwin-site` | ✅ Live at wondertwin.ai |
| Static YAML registry | ✅ Working |
| `ci.yml` (go vet, test, build) | ✅ On main |
| `release.yml` (hand-rolled matrix) | ✅ On main, to be replaced by GoReleaser |
| README.md | ✅ On main |
| `skills/twin-generator.md` | ✅ On main, updated for twinkit |
| CONTRIBUTING.md | 🔄 PR #12 |
| Issue templates (twin-request) | 🔄 PR #12 |
| LICENSE (Apache 2.0) | 🔄 PR #12 |
| CODE_OF_CONDUCT.md | 🔄 PR #12 |
| SECURITY.md | 🔄 PR #12 |
| PR template | 🔄 PR #12 |
| `.goreleaser.yml` | ⏳ Friend's PR (not yet submitted) |
| `homebrew-tap` repo | ⬜ After GoReleaser lands |
| Install script | ⬜ After GoReleaser lands |
| `docs/` folder | ⬜ Not blocking launch |
| Registry auto-update workflow | ⬜ After GoReleaser lands, manual until then |

### Merge sequence to go public

1. **Merge PR #12** — governance files (LICENSE, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, PR template, issue templates)
2. **Friend merges GoReleaser PR** — replaces hand-rolled release.yml with `.goreleaser.yml`
3. **Set up secrets** — org-level `REGISTRY_UPDATE_TOKEN`, repo-level `HOMEBREW_TAP_TOKEN`
4. **Create `homebrew-tap` repo** — empty repo with initial formula
5. **Tag `v0.1.0`** — verify GoReleaser produces clean release
6. **Stamp `.goreleaser.yml` + `release.yml` into all 6 twin repos** — tag each twin
7. **Flip repos to public**
8. **Post launch announcement**

---

## Scaling Considerations

At 1000+ twin repos:

- **Registry performance** — static YAML won't scale. Migrate to API-backed registry that handles concurrent updates.
- **Dependabot noise** — `twinkit` updates across 1000 repos generate 1000 PRs. May need a custom tool to batch-update first-party twins.
- **Template drift** — `.goreleaser.yml`, `release.yml`, or Makefile changes need to propagate to all repos. Consider a repo scaffolding tool or a workflow that opens PRs to update templates across all first-party twin repos.
- **Namespace collisions** — twin naming needs governance. Registry should enforce unique names and possibly namespacing (e.g., `wondertwin/stripe` vs. `acme-corp/stripe`).
- **Binary hosting costs** — GitHub Releases is free for public repos. Private/commercial twins will have bandwidth limits. May need to mirror to S3/CloudFront at scale.
- **GoReleaser Pro** — at very high scale, Pro features like custom publishers and more advanced hooks may be worth the cost. Evaluate when the OSS edition becomes limiting.

These are future problems. The architecture above handles the first 100+ twins without modification.
