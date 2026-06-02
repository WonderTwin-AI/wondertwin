# WonderTwin Verify Action

CI gate that asserts every twin in your `wondertwin-lock.json` is covered
by your org's WonderTwin entitlements. Per
[adr-ci-twin-verification](https://github.com/WonderTwin-AI/wondertwin-docs/blob/main/adr/adr-ci-twin-verification.md),
this is the boundary that converts local exploration into a commercial
conversation — local twin runs are unconstrained; the conversion moment
is when code ships.

## Usage

```yaml
- uses: wondertwin-ai/wondertwin/.github/actions/verify@v1
  with:
    api-key: ${{ secrets.WONDERTWIN_API_KEY }}
```

That's the whole integration. The action:

1. Downloads the `wt` CLI for the runner's OS/arch (default: latest release).
2. Verifies the binary against the release's `checksums.txt`.
3. Runs `wt verify` against `wondertwin-lock.json`, resolving the
   account from `.wondertwin/project.json#account_id` (or an explicit
   `account-id` input).
4. Exits non-zero if any twin in the lockfile is missing entitlements.

## Inputs

| Input               | Required | Default     | Description |
|---------------------|----------|-------------|-------------|
| `api-key`           | yes      | —           | WonderTwin API key. Always pass via secret. |
| `account-id`        | no       | from `project.json` | Override the account_id used for the coverage check. |
| `working-directory` | no       | `.`         | Directory containing `wondertwin-lock.json`. |
| `wt-version`        | no       | `latest`    | Specific `wt` release tag (e.g. `v0.42.0`) for reproducible CI. |
| `base-url`          | no       | production  | Override platform base URL (staging / self-hosted). |

## Exit codes

The action surfaces the underlying `wt verify` exit code:

- **0** — every twin in the lockfile is covered.
- **1** — one or more twins are missing entitlements. The CI job fails;
  the log lists each missing twin with a one-line reason and the
  `wt subscribe <twin>` command to resolve it.
- **2** — configuration error (no lockfile, no API key, network failure).

## Pinning

For reproducible CI, pin both the action and the CLI:

```yaml
- uses: wondertwin-ai/wondertwin/.github/actions/verify@<sha>
  with:
    api-key: ${{ secrets.WONDERTWIN_API_KEY }}
    wt-version: v0.42.0
```

## Platform support

Linux and macOS GitHub-hosted runners (`ubuntu-*`, `macos-*`) on x64
and arm64. Windows is not supported in v1 — open an issue if you need
it.
