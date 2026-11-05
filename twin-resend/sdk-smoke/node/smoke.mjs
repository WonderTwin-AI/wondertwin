// SDK smoke suite: runs the Resend app emulator's email flows through the
// official resend-node SDK (resend 6.30.0, pinned by package-lock.json)
// against a running binary. Test-only; run.sh installs it in a scratch copy.
//
//   RESEND_EMULATOR_URL=http://localhost:4113 node smoke.mjs
//
// Prints one JSON line per case ({sdk, case, pass, detail}) and exits non-zero
// if any case fails. Case names match python/smoke.py.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const base = (process.env.RESEND_EMULATOR_URL ?? "http://localhost:4113").replace(/\/$/, "");
// resend-node reads its base URL from RESEND_BASE_URL when the module loads.
process.env.RESEND_BASE_URL = base;
const { Resend } = await import("resend");

const pkg = JSON.parse(readFileSync(new URL("./node_modules/resend/package.json", import.meta.url), "utf8"));
assert.equal(pkg.version, "6.30.0", "expected resend 6.30.0");

const SDK = "resend-node 6.30.0";
const SENDER = "Smoke <onboarding@resend.dev>";
const resend = new Resend("re_sdkSmoke000000000000000000");
const results = [];

async function check(name, fn) {
  await fetch(base + "/admin/reset", { method: "POST" });
  try {
    await fn();
    results.push({ sdk: SDK, case: name, pass: true, detail: "" });
  } catch (e) {
    results.push({ sdk: SDK, case: name, pass: false, detail: String(e?.message ?? e).replace(/\s+/g, " ").slice(0, 300) });
  }
}

// resend-node returns {data, error} instead of throwing on an API error.
function ok({ data, error }) {
  assert.equal(error, null, `API error: ${JSON.stringify(error)}`);
  return data;
}

await check("send with an array recipient", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: ["delivered@resend.dev"], subject: "smoke", html: "<p>smoke</p>",
  }));
  assert.ok(sent?.id, `no id in ${JSON.stringify(sent)}`);
});

await check("send, then get by id", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: ["delivered@resend.dev"], subject: "smoke get", html: "<p>get</p>",
  }));
  const got = ok(await resend.emails.get(sent.id));
  assert.equal(got.id, sent.id);
  assert.equal(got.object, "email");
  assert.equal(got.subject, "smoke get");
  assert.deepEqual(got.to, ["delivered@resend.dev"]);
});

for (const r of results) console.log(JSON.stringify(r));
process.exit(results.length && results.every((r) => r.pass) ? 0 : 1);
