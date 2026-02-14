# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in WonderTwin, please report it responsibly.

**Email:** security@wondertwin.ai

Please include:
- Description of the vulnerability
- Steps to reproduce
- Affected versions (if known)

We will acknowledge your report within 48 hours and aim to release a fix within 7 days of confirmation.

## Scope

WonderTwin twins are designed for **local development and testing**. They are not production services and should never be exposed to the public internet. That said, we take security seriously in:

- The `wt` CLI (command injection, path traversal, etc.)
- The shared `twinkit` libraries
- The registry and binary distribution pipeline (supply chain integrity)
- GitHub Actions workflows (secret exposure, injection)

## Supported Versions

We support the latest release. There is no LTS or backporting policy at this time.
