# CLAUDE.md — wondertwin

## Twin Coverage Policy

The goal for all twins is **100% API parity** with the real service. Every endpoint, every parameter, every error shape.

**You may never make scope decisions.** Do not decide what is "niche", "low priority", or "not worth implementing". If the real API supports it, the twin must support it.

If you believe there is a legitimate reason to consider cutting or deferring scope, you must surface it using the template below. The decision is never yours — bring it to me.

### Scope Exception Request Template

When you encounter a potential reason to not cover some part of an API surface, present it like this:

```
## Scope Exception Request: [twin-name] — [feature/endpoint]

**What:** [Exact endpoint or behavior from the real API]

**Why consider excluding:**
- [Technical blocker, dependency, or constraint]

**Impact of not covering:**
- [Who uses this? What SDK paths hit it? What tests would be uncoverable?]

**Effort to cover:**
- [Rough size: trivial / small / medium / large]
- [Any new twinkit infrastructure required?]

**Recommendation:** [Cover now / Defer until X / Needs discussion because Y]
```

Do not skip this process. Do not silently omit endpoints. If you're unsure whether something is in the real API's surface, research it.
