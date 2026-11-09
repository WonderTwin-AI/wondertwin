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
await check("create and get a domain", async () => {
  const created = ok(await resend.domains.create({ name: "smoke-shape.example.com" }));
  assert.equal(created.status, "not_started");
  assert.deepEqual(created.records.map((r) => r.name), ["resend._domainkey", "send", "send", "rsend"]);
  const got = ok(await resend.domains.get(created.id));
  assert.equal(got.name, created.name);
  assert.deepEqual(got.capabilities, { sending: "enabled", receiving: "disabled" });
  assert.equal(got.open_tracking, false);
});
await check("update, verify and delete a domain", async () => {
  const created = ok(await resend.domains.create({ name: "smoke-update.example.com" }));
  const updated = ok(await resend.domains.update({ id: created.id, openTracking: true, tls: "enforced" }));
  assert.deepEqual(updated, { object: "domain", id: created.id });
  const got = ok(await resend.domains.get(created.id));
  assert.equal(got.open_tracking, true);
  assert.equal(got.click_tracking, false);
  assert.deepEqual(ok(await resend.domains.verify(created.id)), { object: "domain", id: created.id });
  assert.deepEqual(ok(await resend.domains.remove(created.id)), { object: "domain", id: created.id, deleted: true });
  const { error } = await resend.domains.get(created.id);
  assert.equal(error?.name, "not_found");
});
await check("create, rename and delete an API key", async () => {
  const created = ok(await resend.apiKeys.create({ name: "smoke-key" }));
  const renamed = ok(await resend.apiKeys.update(created.id, { name: "smoke-key-renamed" }));
  assert.deepEqual(renamed, { object: "api_key", id: created.id });
  const keys = ok(await resend.apiKeys.list());
  assert.deepEqual(keys.data.map((k) => k.name), ["smoke-key-renamed"]);
  ok(await resend.apiKeys.remove(created.id));
});
await check("register a webhook and read its secret", async () => {
  const created = ok(await resend.webhooks.create({ endpoint: "https://example.com/smoke-hook", events: ["email.sent", "email.delivered"] }));
  assert.match(created.signing_secret, /^whsec_/);
  const got = ok(await resend.webhooks.get(created.id));
  assert.equal(got.status, "enabled");
  assert.equal(got.signing_secret, created.signing_secret);
  const list = ok(await resend.webhooks.list());
  assert.deepEqual(list.data.map((h) => h.id), [created.id]);
});
await check("disable, change and delete a webhook", async () => {
  const created = ok(await resend.webhooks.create({ endpoint: "https://example.com/smoke-hook", events: ["email.sent"] }));
  const updated = ok(await resend.webhooks.update(created.id, { status: "disabled", endpoint: "https://example.com/smoke-hook-2", events: ["email.bounced"] }));
  assert.deepEqual(updated, { object: "webhook", id: created.id });
  const got = ok(await resend.webhooks.get(created.id));
  assert.equal(got.status, "disabled");
  assert.deepEqual(got.events, ["email.bounced"]);
  assert.deepEqual(ok(await resend.webhooks.remove(created.id)), { object: "webhook", id: created.id, deleted: true });
  const { error } = await resend.webhooks.get(created.id);
  assert.equal(error?.name, "not_found");
});
await check("verify a signed webhook delivery", async () => {
  const { createServer } = await import("node:http");
  let deliver;
  const delivered = new Promise((resolve) => { deliver = resolve; });
  const srv = createServer((req, res) => {
    let body = "";
    req.on("data", (c) => { body += c; });
    req.on("end", () => { res.end("ok"); deliver({ headers: req.headers, body }); });
  });
  await new Promise((resolve) => srv.listen(0, "127.0.0.1", resolve));
  try {
    const hook = ok(await resend.webhooks.create({ endpoint: `http://127.0.0.1:${srv.address().port}/hook`, events: ["email.delivered"] }));
    ok(await resend.emails.send({ from: SENDER, to: "delivered@resend.dev", subject: "smoke hook", html: "<p>h</p>" }));
    const timeout = new Promise((_, reject) => setTimeout(() => reject(new Error("no delivery within 5s")), 5000));
    const { headers, body } = await Promise.race([delivered, timeout]);
    const event = resend.webhooks.verify({
      payload: body,
      headers: { id: headers["svix-id"], timestamp: headers["svix-timestamp"], signature: headers["svix-signature"] },
      webhookSecret: hook.signing_secret,
    });
    assert.equal(event.type, "email.delivered");
  } finally {
    srv.close();
  }
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
await check("list sent emails newest first", async () => {
  const first = ok(await resend.emails.send({ from: SENDER, to: "delivered@resend.dev", subject: "smoke list 1", html: "<p>1</p>" }));
  const second = ok(await resend.emails.send({ from: SENDER, to: "delivered@resend.dev", subject: "smoke list 2", html: "<p>2</p>" }));
  let page = ok(await resend.emails.list());
  assert.deepEqual(page.data.map((e) => e.id), [second.id, first.id]);
  assert.equal(page.has_more, false);
  page = ok(await resend.emails.list({ limit: 1 }));
  assert.deepEqual(page.data.map((e) => e.id), [second.id]);
  assert.equal(page.has_more, true);
});
await check("schedule a send in natural language", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: "delivered@resend.dev", subject: "smoke later", html: "<p>l</p>", scheduledAt: "in 1 hour",
  }));
  const got = ok(await resend.emails.get(sent.id));
  assert.equal(got.last_event, "scheduled");
  assert.ok(got.scheduled_at, `scheduled_at is ${got.scheduled_at}`);
});
await check("reschedule, then cancel a scheduled send", async () => {
  const sent = ok(await resend.emails.send({
    from: SENDER, to: "delivered@resend.dev", subject: "smoke move", html: "<p>m</p>", scheduledAt: "in 1 hour",
  }));
  const moved = ok(await resend.emails.update({ id: sent.id, scheduledAt: "in 2 hours" }));
  assert.deepEqual(moved, { object: "email", id: sent.id });
  const cancelled = ok(await resend.emails.cancel(sent.id));
  assert.deepEqual(cancelled, { object: "email", id: sent.id });
  const got = ok(await resend.emails.get(sent.id));
  assert.equal(got.last_event, "canceled");
});

for (const r of results) console.log(JSON.stringify(r));
process.exit(results.length && results.every((r) => r.pass) ? 0 : 1);
