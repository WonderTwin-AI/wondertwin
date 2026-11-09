// SDK smoke suite: runs the Resend app emulator's email flows through the
// official resend-node SDK (resend 6.30.0, pinned by package-lock.json)
// against a running binary. Test-only; run.sh installs it in a scratch copy.
//
//   RESEND_EMULATOR_URL=http://localhost:4113 node smoke.mjs
//
// Prints one JSON line per case ({sdk, case, pass, detail}) and exits non-zero
// if any case fails. Case names match python/smoke.py.
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
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
await check("get an unknown id answers not_found", async () => {
  const { data, error } = await resend.emails.get("4ef9a417-02e9-4d39-ad75-9611e0fcc33c");
  assert.equal(data, null);
  assert.equal(error?.name, "not_found", `error is ${JSON.stringify(error)}`);
  assert.equal(error?.statusCode, 404, `error is ${JSON.stringify(error)}`);
});

await check("send with a string recipient", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: "delivered@resend.dev", subject: "smoke string", html: "<p>string</p>",
  }));
  const got = ok(await resend.emails.get(sent.id));
  assert.deepEqual(got.to, ["delivered@resend.dev"]);
});

await check("send with tags, then get them back", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: ["delivered@resend.dev"], subject: "smoke tags", html: "<p>tags</p>",
    tags: [{ name: "category", value: "smoke" }],
  }));
  const got = ok(await resend.emails.get(sent.id));
  assert.deepEqual(got.tags, [{ name: "category", value: "smoke" }]);
});

await check("get returns Resend's read fields", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: "delivered@resend.dev", subject: "smoke fields", html: "<p>fields</p>",
  }));
  const got = ok(await resend.emails.get(sent.id));
  for (const k of ["object", "id", "to", "from", "created_at", "subject", "bcc", "cc", "reply_to",
    "last_event", "scheduled_at", "message_id", "html", "text"]) {
    assert.ok(k in got, `missing ${k}`);
  }
  assert.ok(!("status" in got), "status is not a field Resend returns");
  assert.match(got.message_id, /@email\.amazonses\.com>$/);
  assert.equal(got.text, "fields");
});

const batchItems = () => [
  { from: SENDER, to: "delivered@resend.dev", subject: "smoke batch", html: "<p>b</p>" },
  { to: "delivered@resend.dev", subject: "smoke batch", html: "<p>b</p>" },
];

await check("batch: strict fails on an invalid item", async () => {
  const { data, error } = await resend.batch.send(batchItems());
  assert.equal(data, null);
  assert.equal(error?.statusCode, 422, `error is ${JSON.stringify(error)}`);
});

await check("batch: permissive sends the valid items", async () => {
  const got = ok(await resend.batch.send(batchItems(), { batchValidation: "permissive" }));
  assert.equal(got.data.length, 1, JSON.stringify(got));
  assert.deepEqual(got.errors, [{ index: 1, message: "Missing `from` field." }]);
});

await check("idempotent retry returns the first send", async () => {
  const idempotencyKey = `smoke-${randomUUID()}`;
  const params = { from: SENDER, to: "delivered@resend.dev", subject: "smoke idem", html: "<p>i</p>" };
  const first = ok(await resend.emails.send(params, { idempotencyKey }));
  const again = ok(await resend.emails.send(params, { idempotencyKey }));
  assert.equal(again.id, first.id);
  const { data, error } = await resend.emails.send({ ...params, subject: "changed" }, { idempotencyKey });
  assert.equal(data, null);
  assert.equal(error?.statusCode, 409, `error is ${JSON.stringify(error)}`);
});
await check("a send to bounced@resend.dev reads back bounced", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: "bounced@resend.dev", subject: "smoke bounce", html: "<p>b</p>",
  }));
  const got = ok(await resend.emails.get(sent.id));
  assert.equal(got.last_event, "bounced");
});
await check("a send from an unverified domain answers 403", async () => {
  ok(await resend.domains.create({ name: "smoke-unverified.example.com" }));
  const { data, error } = await resend.emails.send({
    from: "a@smoke-unverified.example.com", to: "delivered@resend.dev", subject: "smoke domain", html: "<p>d</p>",
  });
  assert.equal(data, null);
  assert.equal(error?.statusCode, 403, `error is ${JSON.stringify(error)}`);
  assert.match(error.message, /domain is not verified/);
});
await check("list domains with a limit pages newest first", async () => {
  const first = ok(await resend.domains.create({ name: "smoke-page-one.example.com" }));
  const second = ok(await resend.domains.create({ name: "smoke-page-two.example.com" }));
  let page = ok(await resend.domains.list({ limit: 1 }));
  assert.deepEqual(page.data.map((d) => d.id), [second.id]);
  assert.equal(page.has_more, true);
  page = ok(await resend.domains.list({ limit: 1, after: second.id }));
  assert.deepEqual(page.data.map((d) => d.id), [first.id]);
  assert.equal(page.has_more, false);
});

for (const r of results) console.log(JSON.stringify(r));
process.exit(results.length && results.every((r) => r.pass) ? 0 : 1);
