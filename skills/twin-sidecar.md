# SKILL: WonderTwin Twin Sidecar

## Purpose

Build optional companion processes (sidecars) that enrich a twin with real-world data from the upstream service's API. Sidecars bridge the gap between fully deterministic twin behavior and production-realistic test data, while keeping the twin itself offline and stateless.

A sidecar fetches data from the real API, transforms it into a seed file or pushes it via the admin API, and the twin serves it. The twin never calls the real service — the sidecar does.

## When to Use

- A twin needs realistic data that cannot be generated deterministically (e.g., real logos, product catalogs, email templates, feature flag configs)
- Users want their test environment to mirror production state without manual seed file creation
- The twin already supports `LoadState` / `--seed` but has no tooling to populate the seed

## When NOT to Use

- The twin's deterministic data is sufficient for testing (most twins don't need sidecars)
- The real API requires paid access that users may not have
- The data is static and can ship as a checked-in seed file

## Design Principles

1. **Sidecars are always optional.** The twin must work without the sidecar. Sidecars add realism, not functionality.
2. **The twin stays offline.** Only the sidecar contacts the real API. The twin never makes outbound requests.
3. **Users provide their own credentials.** The sidecar uses the user's API key for the real service. This avoids licensing issues with redistributing fetched data.
4. **Output is a seed file.** The sidecar produces a JSON file compatible with the twin's `LoadState` format. Users can inspect, edit, and version-control the output.
5. **Idempotent and cacheable.** Running the sidecar twice with the same input produces the same seed file. Include timestamps and checksums for cache invalidation.

## Directory Structure

Sidecars live in the twin's directory under `sidecar/`:

```
twin-{name}/
├── cmd/twin-{name}/main.go
├── internal/...
├── sidecar/
│   └── {function}/
│       ├── main.go           # Entry point
│       └── README.md         # Usage, required env vars, output format
├── twin-manifest.json
└── twin.json
```

Example for logodev:
```
twin-logodev/
├── sidecar/
│   └── fetch-logos/
│       ├── main.go
│       └── README.md
```

## Naming Convention

- Directory: `sidecar/{function}/` (e.g., `fetch-logos`, `sync-flags`, `mirror-catalog`)
- Binary: `sidecar-{twin}-{function}` (e.g., `sidecar-logodev-fetch-logos`)
- The function name should describe what data the sidecar fetches, not what it does with it

## Implementation Steps

### 1. Define the Seed Format

The sidecar's output must match the twin's `LoadState` snapshot format. Check the twin's `stateSnapshot` struct in `internal/store/memory.go` to understand what fields are expected.

```go
// Example: logodev seed format
{
  "custom_logos": {
    "stripe.com": {
      "content_type": "image/svg+xml",
      "data": "<base64-encoded content>"
    }
  }
}
```

### 2. Build the Fetcher

The sidecar is a standalone Go binary that:
- Reads configuration from environment variables (API keys, domain lists, etc.)
- Calls the real service API
- Transforms responses into the twin's seed format
- Writes the seed file to stdout or a specified path

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    apiKey := os.Getenv("SERVICE_API_KEY")
    if apiKey == "" {
        fmt.Fprintf(os.Stderr, "SERVICE_API_KEY required\n")
        os.Exit(1)
    }

    // Fetch from real API...
    // Transform to seed format...
    // Write to stdout for piping into twin
    json.NewEncoder(os.Stdout).Encode(seed)
}
```

### 3. Document Usage

Every sidecar must have a `README.md` that covers:
- What data it fetches
- Required environment variables
- Example usage
- Output format description
- Rate limiting / API quota considerations
- Licensing notes (can the fetched data be redistributed?)

### 4. Wire to the Twin

Users can use the sidecar output in two ways:

**Via seed file (startup):**
```bash
# Fetch and save
sidecar-logodev-fetch-logos > logos-seed.json

# Start twin with seed
twin-logodev --seed logos-seed.json
```

**Via admin API (runtime):**
```bash
# Twin is already running
sidecar-logodev-fetch-logos | curl -X POST http://localhost:4116/admin/state -d @-
```

### 5. Add to wondertwin.json (Optional)

If the user wants the sidecar to run alongside the twin:
```json
{
  "twins": {
    "logodev": {
      "binary": "./bin/twin-logodev",
      "port": 4116,
      "seed": "./seeds/logos.json"
    }
  }
}
```

The sidecar runs separately to refresh the seed file. It is not managed by `wt up`.

## Quality Checklist

- [ ] **Sidecar is optional.** Twin works without it.
- [ ] **No credentials in code.** API keys come from environment variables.
- [ ] **Output matches LoadState format.** The seed file loads without errors.
- [ ] **README documents everything.** Env vars, usage, output format, licensing.
- [ ] **Idempotent.** Same input produces same output.
- [ ] **Error handling.** Clear messages for missing credentials, rate limits, API errors.
- [ ] **No fetched data is committed.** Seed files generated by sidecars are in `.gitignore` or documented as user-generated.

## Common Sidecar Patterns

| Pattern | Example | Description |
|---------|---------|-------------|
| **Asset fetcher** | Logo images, templates | Downloads binary or text assets and encodes them for the seed file |
| **Config mirror** | Feature flags, settings | Fetches configuration state to replicate production behavior |
| **Catalog sync** | Products, prices, plans | Mirrors a subset of a service's data catalog |
| **Schema fetcher** | Webhook schemas, event types | Downloads schema definitions the twin needs to validate against |

## Licensing Considerations

Sidecars fetch data from real APIs. The fetched data may be subject to the service's terms of use. Key points to document in each sidecar's README:

- Whether the service allows caching/storing API responses
- Whether fetched data can be redistributed (usually no — users fetch their own)
- Rate limits and API quota impact
- Whether the sidecar requires a paid API tier

The default assumption: **users fetch their own data with their own credentials.** Sidecars are tools, not data distributors.
