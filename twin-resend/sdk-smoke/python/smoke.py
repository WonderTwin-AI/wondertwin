"""SDK smoke suite: runs the Resend app emulator's email flows through the
official resend-python SDK (resend 2.48.0, pinned by uv.lock) against a
running binary.

    RESEND_EMULATOR_URL=http://localhost:4113 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails. Case names match node/smoke.mjs so run.sh can
put both SDKs in one table.
"""

import json
import os
import sys
import uuid

import requests

BASE = os.environ.get("RESEND_EMULATOR_URL", "http://localhost:4113").rstrip("/")
# resend-python reads its base URL from RESEND_API_URL at import time.
os.environ["RESEND_API_URL"] = BASE

import resend  # noqa: E402

SDK = "resend-python 2.48.0"
resend.api_key = "re_sdkSmoke000000000000000000"
SENDER = "Smoke <onboarding@resend.dev>"

results = []


def admin(method, path):
    resp = requests.request(method, BASE + path, timeout=10)
    resp.raise_for_status()
    return resp.content


def case(name):
    def wrap(fn):
        admin("POST", "/admin/reset")
        try:
            fn()
            results.append({"sdk": SDK, "case": name, "pass": True, "detail": ""})
        except Exception as e:  # noqa: BLE001 - every failure becomes a row
            results.append({"sdk": SDK, "case": name, "pass": False, "detail": f"{type(e).__name__}: {e}"})
        return fn
    return wrap


@case("send with an array recipient")
def _():
    sent = resend.Emails.send({
        "from": SENDER, "to": ["delivered@resend.dev"], "subject": "smoke", "html": "<p>smoke</p>",
    })
    assert sent.get("id"), f"no id in {sent!r}"


@case("send, then get by id")
def _():
    sent = resend.Emails.send({
        "from": SENDER, "to": ["delivered@resend.dev"], "subject": "smoke get", "html": "<p>get</p>",
    })
    got = resend.Emails.get(sent["id"])
    assert got["id"] == sent["id"], f"get returned id {got.get('id')!r}, sent {sent['id']!r}"
    assert got.get("object") == "email", f"object is {got.get('object')!r}"
    assert got.get("subject") == "smoke get", f"subject is {got.get('subject')!r}"
    assert got.get("to") == ["delivered@resend.dev"], f"to is {got.get('to')!r}"



@case("get an unknown id answers not_found")
def _():
    try:
        resend.Emails.get("4ef9a417-02e9-4d39-ad75-9611e0fcc33c")
    except resend.exceptions.ResendError as e:
        assert str(e.code) == "404", f"code is {e.code!r}"
        assert e.error_type == "not_found", f"error_type is {e.error_type!r}"
        return
    raise AssertionError("get of an unknown id did not raise")



@case("send with a string recipient")
def _():
    sent = resend.Emails.send({
        "from": SENDER, "to": "delivered@resend.dev", "subject": "smoke string", "html": "<p>string</p>",
    })
    got = resend.Emails.get(sent["id"])
    assert got.get("to") == ["delivered@resend.dev"], f"to is {got.get('to')!r}"


@case("send with tags, then get them back")
def _():
    sent = resend.Emails.send({
        "from": SENDER, "to": ["delivered@resend.dev"], "subject": "smoke tags", "html": "<p>tags</p>",
        "tags": [{"name": "category", "value": "smoke"}],
    })
    got = resend.Emails.get(sent["id"])
    assert got.get("tags") == [{"name": "category", "value": "smoke"}], f"tags is {got.get('tags')!r}"



@case("get returns Resend's read fields")
def _():
    sent = resend.Emails.send({
        "from": SENDER, "to": "delivered@resend.dev", "subject": "smoke fields", "html": "<p>fields</p>",
    })
    got = resend.Emails.get(sent["id"])
    for k in ("object", "id", "to", "from", "created_at", "subject", "bcc", "cc", "reply_to",
              "last_event", "scheduled_at", "message_id", "html", "text"):
        assert k in got, f"missing {k}"
    assert "status" not in got, "status is not a field Resend returns"
    assert got["message_id"].endswith("@email.amazonses.com>"), f"message_id is {got['message_id']!r}"
    assert got["text"] == "fields", f"text is {got['text']!r}"



def _batch_items():
    good = {"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke batch", "html": "<p>b</p>"}
    bad = {"to": "delivered@resend.dev", "subject": "smoke batch", "html": "<p>b</p>"}
    return [good, bad]


@case("batch: strict fails on an invalid item")
def _():
    try:
        resend.Batch.send(_batch_items())
    except resend.exceptions.ResendError as e:
        assert str(e.code) == "422", f"code is {e.code!r}"
        return
    raise AssertionError("strict batch with an invalid item did not raise")


@case("batch: permissive sends the valid items")
def _():
    got = resend.Batch.send(_batch_items(), {"batch_validation": "permissive"})
    assert len(got["data"]) == 1, f"data is {got.get('data')!r}"
    assert got["errors"] == [{"index": 1, "message": "Missing `from` field."}], f"errors is {got.get('errors')!r}"


@case("idempotent retry returns the first send")
def _():
    key = f"smoke-{uuid.uuid4()}"
    params = {"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke idem", "html": "<p>i</p>"}
    first = resend.Emails.send(params, {"idempotency_key": key})
    again = resend.Emails.send(params, {"idempotency_key": key})
    assert first["id"] == again["id"], f"ids differ: {first['id']} vs {again['id']}"
    try:
        resend.Emails.send({**params, "subject": "changed"}, {"idempotency_key": key})
    except resend.exceptions.ResendError as e:
        assert str(e.code) == "409", f"code is {e.code!r}"
        return
    raise AssertionError("same key with another body did not raise")


@case("a send to bounced@resend.dev reads back bounced")
def _():
    sent = resend.Emails.send({"from": SENDER, "to": "bounced@resend.dev", "subject": "smoke bounce", "html": "<p>b</p>"})
    got = resend.Emails.get(sent["id"])
    assert got["last_event"] == "bounced", f"last_event is {got['last_event']!r}"


@case("a send from an unverified domain answers 403")
def _():
    resend.Domains.create({"name": "smoke-unverified.acme.dev"})
    try:
        resend.Emails.send({"from": "a@smoke-unverified.acme.dev", "to": "delivered@resend.dev",
                            "subject": "smoke domain", "html": "<p>d</p>"})
    except resend.exceptions.ResendError as e:
        assert str(e.code) == "403", f"code is {e.code!r}"
        assert "domain is not verified" in str(e), f"message is {e}"
        return
    raise AssertionError("send from an unverified domain did not raise")



@case("create and get a domain")
def _():
    created = resend.Domains.create({"name": "smoke-shape.acme.dev"})
    assert created["status"] == "not_started", f"create is {created!r}"
    names = [r["name"] for r in created["records"]]
    assert names == ["resend._domainkey", "send", "send", "rsend"], f"records are {names!r}"
    got = resend.Domains.get(created["id"])
    assert got["name"] == created["name"], f"get is {got!r}"
    assert got["capabilities"] == {"sending": "enabled", "receiving": "disabled"}, f"get is {got!r}"
    assert got["click_tracking"] is False, f"get is {got!r}"


@case("update, verify and delete a domain")
def _():
    created = resend.Domains.create({"name": "smoke-update.acme.dev"})
    updated = resend.Domains.update({"id": created["id"], "open_tracking": True, "tls": "enforced"})
    assert (updated["object"], updated["id"]) == ("domain", created["id"]), f"update is {updated!r}"
    got = resend.Domains.get(created["id"])
    assert got["open_tracking"] is True and got["click_tracking"] is False, f"get is {got!r}"
    verified = resend.Domains.verify(created["id"])
    assert (verified["object"], verified["id"]) == ("domain", created["id"]), f"verify is {verified!r}"
    removed = resend.Domains.remove(created["id"])
    assert (removed["id"], removed["deleted"]) == (created["id"], True), f"remove is {removed!r}"
    try:
        resend.Domains.get(created["id"])
    except Exception as e:
        assert "Domain not found" in str(e), f"message is {e}"
        return
    raise AssertionError("get after remove did not raise")


@case("create, rename and delete an API key")
def _():
    created = resend.ApiKeys.create({"name": "smoke-key"})
    renamed = resend.ApiKeys.update({"id": created["id"], "name": "smoke-key-renamed"})
    assert (renamed["object"], renamed["id"]) == ("api_key", created["id"]), f"update is {renamed!r}"
    keys = resend.ApiKeys.list()
    assert [k["name"] for k in keys["data"]] == ["smoke-key-renamed"], f"list is {keys!r}"
    resend.ApiKeys.remove(created["id"])


@case("register a webhook and read its secret")
def _():
    created = resend.Webhooks.create({"endpoint": "https://example.com/smoke-hook", "events": ["email.sent", "email.delivered"]})
    assert created["signing_secret"].startswith("whsec_"), f"create is {created!r}"
    got = resend.Webhooks.get(created["id"])
    assert got["status"] == "enabled" and got["signing_secret"] == created["signing_secret"], f"get is {got!r}"
    hooks = resend.Webhooks.list()
    assert [h["id"] for h in hooks["data"]] == [created["id"]], f"list is {hooks!r}"


@case("disable, change and delete a webhook")
def _():
    created = resend.Webhooks.create({"endpoint": "https://example.com/smoke-hook", "events": ["email.sent"]})
    updated = resend.Webhooks.update({"webhook_id": created["id"], "status": "disabled",
                                      "endpoint": "https://example.com/smoke-hook-2", "events": ["email.bounced"]})
    assert (updated["object"], updated["id"]) == ("webhook", created["id"]), f"update is {updated!r}"
    got = resend.Webhooks.get(created["id"])
    assert got["status"] == "disabled" and got["events"] == ["email.bounced"], f"get is {got!r}"
    removed = resend.Webhooks.remove(created["id"])
    assert (removed["id"], removed["deleted"]) == (created["id"], True), f"remove is {removed!r}"
    try:
        resend.Webhooks.get(created["id"])
    except Exception as e:
        assert "Webhook endpoint not found" in str(e), f"message is {e}"
        return
    raise AssertionError("get after remove did not raise")


@case("verify a signed webhook delivery")
def _():
    import http.server
    import queue
    import threading

    got = queue.Queue()

    class Hook(http.server.BaseHTTPRequestHandler):
        def do_POST(self):  # noqa: N802 - the stdlib's name
            body = self.rfile.read(int(self.headers["Content-Length"])).decode()
            self.send_response(200)
            self.end_headers()
            got.put((dict(self.headers), body))

        def log_message(self, *args):
            pass

    srv = http.server.HTTPServer(("127.0.0.1", 0), Hook)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    try:
        hook = resend.Webhooks.create({"endpoint": f"http://127.0.0.1:{srv.server_port}/hook", "events": ["email.delivered"]})
        resend.Emails.send({"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke hook", "html": "<p>h</p>"})
        headers, body = got.get(timeout=5)
        headers = {k.lower(): v for k, v in headers.items()}
        event = resend.Webhooks.verify({
            "payload": body,
            "headers": {"id": headers["svix-id"], "timestamp": headers["svix-timestamp"], "signature": headers["svix-signature"]},
            "webhook_secret": hook["signing_secret"],
        })
        assert event["type"] == "email.delivered", f"event is {event!r}"
    finally:
        srv.shutdown()


@case("list domains with a limit pages newest first")
def _():
    first = resend.Domains.create({"name": "smoke-page-one.acme.dev"})
    second = resend.Domains.create({"name": "smoke-page-two.acme.dev"})
    page = resend.Domains.list({"limit": 1})
    assert [d["id"] for d in page["data"]] == [second["id"]], f"page 1 is {page!r}"
    assert page["has_more"] is True, f"has_more is {page['has_more']!r}"
    page = resend.Domains.list({"limit": 1, "after": second["id"]})
    assert [d["id"] for d in page["data"]] == [first["id"]], f"page 2 is {page!r}"
    assert page["has_more"] is False, f"has_more is {page['has_more']!r}"


@case("list sent emails newest first")
def _():
    first = resend.Emails.send({"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke list 1", "html": "<p>1</p>"})
    second = resend.Emails.send({"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke list 2", "html": "<p>2</p>"})
    page = resend.Emails.list()
    assert [e["id"] for e in page["data"]] == [second["id"], first["id"]], f"list is {page!r}"
    assert page["has_more"] is False, f"has_more is {page['has_more']!r}"
    page = resend.Emails.list({"limit": 1})
    assert [e["id"] for e in page["data"]] == [second["id"]] and page["has_more"] is True, f"limit 1 is {page!r}"


@case("schedule a send in natural language")
def _():
    sent = resend.Emails.send({"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke later",
                               "html": "<p>l</p>", "scheduled_at": "in 1 hour"})
    got = resend.Emails.get(sent["id"])
    assert got["last_event"] == "scheduled", f"last_event is {got['last_event']!r}"
    assert got["scheduled_at"], f"scheduled_at is {got['scheduled_at']!r}"


@case("reschedule, then cancel a scheduled send")
def _():
    sent = resend.Emails.send({"from": SENDER, "to": "delivered@resend.dev", "subject": "smoke move",
                               "html": "<p>m</p>", "scheduled_at": "in 1 hour"})
    moved = resend.Emails.update({"id": sent["id"], "scheduled_at": "in 2 hours"})
    assert moved["id"] == sent["id"] and moved["object"] == "email", f"update answered {moved!r}"
    cancelled = resend.Emails.cancel(sent["id"])
    assert cancelled["id"] == sent["id"] and cancelled["object"] == "email", f"cancel answered {cancelled!r}"
    got = resend.Emails.get(sent["id"])
    assert got["last_event"] == "canceled", f"last_event is {got['last_event']!r}"

for r in results:
    print(json.dumps(r))
sys.exit(0 if results and all(r["pass"] for r in results) else 1)
