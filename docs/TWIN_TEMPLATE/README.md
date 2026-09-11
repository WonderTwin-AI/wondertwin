# twin-TEMPLATE

> **Using this template: delete `go.mod` when you copy it.**
>
> The template carries its own `go.mod` for one reason: it makes this
> directory a separate module, so `go build ./...` at the repo root does not
> try to compile `twin-TEMPLATE` placeholders. Dependabot tracks it from
> there too.
>
> A real emulator has no `go.mod`. All ten community emulators are part of
> the root module, which is what makes the engine boundary enforceable by the
> compiler: the root `go.mod` replaces `twinkit` with `./twinkit`, so an
> emulator importing a package that is not in the public kit fails to build.
> Give an emulator its own `go.mod` and it resolves from the module proxy
> instead, where `twinkit@v0.1.0` still serves every domain engine.
>
> `scripts/check-twinkit-resolution` fails CI on any `twin-*/go.mod`, so this
> is caught rather than discovered later.

WonderTwin behavioral twin for the **Your Service** API.

## Covered Resources

| Resource | Create | Read | Update | Delete | List |
|----------|--------|------|--------|--------|------|
| Resources | Yes | Yes | Yes | Yes | Yes |

## SDK Compatibility

| SDK | Language | Version | Status |
|-----|----------|---------|--------|
| `github.com/your-org/your-sdk-go` | Go | v1 | Primary target |

## Quick Start

```bash
# Build
go build -o twin-TEMPLATE ./cmd/twin-TEMPLATE

# Run
./twin-TEMPLATE --port 4200

# Or with seed data
./twin-TEMPLATE --port 4200 --seed seed.json
```

Point the official SDK client at `http://localhost:4200` and use any API key.

## Known Limitations

- List any endpoints that are stubbed or partially implemented.
- List any SDK behaviors that differ from production.
- List any quirks discovered during development.

## Admin Endpoints

All twins expose the standard WonderTwin admin API:

- `GET /admin/health` - Health check
- `POST /admin/reset` - Reset all state
- `GET /admin/state` - Snapshot current state
- `POST /admin/state/load` - Load state from snapshot
- `POST /admin/fault` - Configure fault injection
- `POST /admin/time` - Simulate time advancement
