## New Twin: twin-{name}

**Service:** {Service Name}
**SDK target:** `{sdk_package}` ({language}, {version})
**Category:** {category}

### Description

<!-- Brief description of what this twin does and which parts of the service it covers. -->

### Resources Covered

<!-- List the SDK resources this twin implements. -->

-

### Checklist

- [ ] `twin-manifest.json` present and validates against `schemas/twin-manifest.schema.json`
- [ ] `provenance.json` present and validates against `schemas/provenance.schema.json`
- [ ] Standard directory structure followed (`cmd/`, `internal/api/`, `internal/store/`)
- [ ] Admin API conformance passes (`wt conformance`)
- [ ] Handler tests pass (`go test ./...`)
- [ ] At least one seed data example exists (JSON file or inline in tests)
- [ ] README documents covered resources and known limitations
- [ ] Auth middleware matches the real service's auth pattern
- [ ] Error responses match the real service's error format
- [ ] SDK client works against the twin without modification

### Testing Notes

<!-- How did you verify SDK compatibility? What did you test manually vs. automated? -->

### Known Limitations

<!-- Any endpoints that are stubbed, partially implemented, or have known behavioral gaps. -->
