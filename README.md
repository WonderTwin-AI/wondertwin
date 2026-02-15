<p align="center">
  <img src="assets/twin-pair-fist-bump.png" alt="WonderTwin" width="200" />
</p>

<h1 align="center">WonderTwin</h1>

<p align="center">
  <strong>Stop mocking. Start cloning.</strong><br/>
  Open-source behavioral API twins for local dev, CI, and chaos testing.
</p>

<p align="center">
  <a href="https://wondertwin.ai">Website</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#twin-catalog">Catalog</a> ·
  <a href="#how-it-works">How It Works</a> ·
  <a href="#contributing">Contributing</a>
</p>

---

WonderTwin gives you behavioral replicas of third-party APIs that run on your machine. Not mocks that return canned responses. Not stubs that match schemas. Twins — stateful, behavioral clones that track balances, fire webhooks, manage sessions, and break the way real APIs break.

Point your SDK at localhost. Your existing tests just work.

```bash
wt install stripe@latest twilio@latest clerk@latest
wt up
# ✓ stripe running on :4111
# ✓ twilio running on :4112
# ✓ clerk  running on :4113
```

## Why

Every team that integrates third-party APIs hits the same problems:

- **Mocks lie.** They return the right shapes but wrong behaviors. They don't maintain state, fire webhooks, or enforce business rules.
- **Sandboxes are limited.** Rate-limited, online-only, incomplete. Can't test offline. Can't simulate failures.
- **Integration tests flake.** Not because your code is broken — because Stripe had a blip or Twilio was slow.
- **You find real bugs in production.** What happens when a payment provider returns 429s during peak checkout? You find out when your customers find out.

WonderTwin fixes this. Run the full dependency stack on your laptop, on a plane, in CI. Deterministic. Fast. Offline.

## Quick Start

```bash
# Install the CLI
brew install wondertwin-ai/tap/wt

# Or download a prebuilt binary from GitHub Releases
# https://github.com/WonderTwin-AI/wondertwin/releases
curl -Lo wt https://github.com/WonderTwin-AI/wondertwin/releases/latest/download/wt-darwin-arm64
chmod +x wt && sudo mv wt /usr/local/bin/

# Or build from source
git clone https://github.com/wondertwin-ai/wondertwin.git
cd wondertwin
make build-all
```

Create a `wondertwin.yaml` in your project:

```yaml
twins:
  stripe:
    binary: ./bin/twin-stripe
    port: 4111
  twilio:
    binary: ./bin/twin-twilio
    port: 4112
  clerk:
    binary: ./bin/twin-clerk
    port: 4113
```

Start everything:

```bash
wt up
```

Point your SDK at localhost:

```go
// Change one line — everything else stays the same
stripe.SetBackend(stripe.APIBackend, &stripe.BackendConfig{
    URL: "http://localhost:4111",
})
```

Run your tests as usual:

```bash
go test ./...
```

## How It Works

Each twin is a self-contained Go binary that implements a behavioral clone of a third-party API. Twins maintain state in memory, enforce business logic, fire webhooks, and expose the same endpoints the real API does — so official SDKs work against them without modification.

Every twin also exposes a standard admin API for test control:

```bash
# Reset all state between tests
curl -X POST localhost:4111/admin/reset

# Load seed data
curl -X POST localhost:4111/admin/state -d @fixtures/stripe.json

# Inspect internal state
curl localhost:4111/admin/state

# Health check
curl localhost:4111/admin/health

# Inject a fault (return 500 on transfers 50% of the time)
curl -X POST localhost:4111/admin/fault/v1/transfers \
  -d '{"status_code": 500, "rate": 0.5}'

# Advance simulated time
curl -X POST localhost:4111/admin/time/advance \
  -d '{"duration": "24h"}'
```

Works with any test framework. Go, Python, Node, Rust, Java — if it speaks HTTP, it works with WonderTwin.

## Twin Catalog

| Twin | Coverage | Default Port |
|------|----------|-------------|
| **Stripe** | Accounts, Balance, Transfers, Payouts, External Accounts, Events, Webhooks | 4111 |
| **Twilio** | Messages, Verify (OTP send/check) | 4112 |
| **Clerk** | Users, Sessions, Organizations, JWT validation | 4113 |
| **Resend** | Email send, delivery status | 4114 |
| **PostHog** | Event capture, batch ingestion | 4115 |
| **Logo.dev** | Logo image retrieval | 4116 |

More twins coming. [Request a twin →](https://github.com/wondertwin-ai/wondertwin/issues/new?template=twin-request.yml)

## CLI Reference

| Command | Description |
|---------|-------------|
| `wt up` | Start all twins defined in `wondertwin.yaml` |
| `wt down` | Stop all running twins |
| `wt status` | Show running twins with PID, port, and health |
| `wt reset` | Reset all twin state |
| `wt seed <twin> <file>` | Load seed data into a twin |
| `wt logs <twin>` | Tail a twin's log output |
| `wt install <twin>@<version>` | Install a twin from the registry |

## MCP Server

WonderTwin includes an MCP server for AI coding agents. Agents can discover, install, start, seed, and inspect twins programmatically.

```bash
wt mcp
```

This enables agentic development workflows where your coding agent has full access to behavioral API twins for testing the code it writes.

## Project Structure

```
wondertwin/
├── cmd/wt/                    # CLI entry point
├── pkg/
│   ├── twincore/              # Server scaffolding, middleware, response helpers
│   ├── store/                 # Generic in-memory store, simulated clock
│   ├── admin/                 # Standard admin API handler
│   ├── webhook/               # Outbound webhook dispatcher
│   └── testutil/              # Test client helpers
├── twin-stripe/               # Stripe behavioral twin
├── twin-twilio/               # Twilio behavioral twin
├── twin-clerk/                # Clerk behavioral twin
├── twin-resend/               # Resend behavioral twin
├── twin-posthog/              # PostHog behavioral twin
├── twin-logodev/              # Logo.dev behavioral twin
├── wondertwin.example.yaml    # Example manifest
└── Makefile
```

## Why Go Binaries?

- **Single file, zero dependencies.** No Docker, no runtime, no package manager conflicts.
- **Cross-platform.** macOS and Linux (ARM and x86). Windows binaries are provided but currently untested.
- **Fast startup.** Twins launch in milliseconds.
- **Deployable anywhere.** Laptop, CI runner, Kubernetes pod, bare metal.
- **Easy to generate.** Go's standard library handles HTTP, JSON, and concurrency out of the box — making twins ideal targets for AI coding agents.

## Contributing

Want to add a twin? Every twin follows the same structure and most can be generated by an AI coding agent in 2-4 hours. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide, or jump straight to the [twin generator skill](skills/twin-generator.md).

[Request a twin →](https://github.com/wondertwin-ai/wondertwin/issues/new?template=twin-request.yml) · [Browse open requests →](https://github.com/wondertwin-ai/wondertwin/issues?q=is%3Aissue+label%3Atwin-request)

## Origin Story

WonderTwin started as internal infrastructure at [Saltwyk](https://saltwyk.com), where we manage 14+ third-party API integrations as a small, AI-native team. Rather than maintaining tests against live APIs — with all the rate limits, flakiness, and sandbox limitations that entails — we built local behavioral replicas of every service we depend on.

It worked so well that we open-sourced it.

## License

Apache 2.0
