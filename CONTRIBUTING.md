# Contributing to WonderTwin

Thank you for the interest. **We are not accepting code contributions right now.**

The architecture these emulators are built on is being reworked, and the build
standard is changing with it. A twin contributed against today's shape would be
rebuilt rather than merged, which is not a fair trade for anyone's afternoon. We
would rather say so than leave a guide up that implies otherwise.

Two things we do want, and act on:

## Request an emulator

[Open a request](https://github.com/wondertwin-ai/wondertwin/issues/new?template=twin-request.yml)
and tell us which service, which SDK operations matter, and how you use that
service in your tests. Requests are the strongest signal we have for what to
build next, and the last part matters most: knowing how you test against a
service today is what tells us whether an emulator can actually fit your
workflow.

## Report a bug in a shipped emulator

If an official SDK behaves differently against one of the emulators here than it
does against the real service, that is a bug worth filing. Include the SDK and
version, the call you made, what the real service returns, and what the emulator
returned. A failing reproduction is the most useful thing you can send us.

---

Contribution will reopen with a documented build process once the architecture
settles. Until then, issues and discussions are read.
