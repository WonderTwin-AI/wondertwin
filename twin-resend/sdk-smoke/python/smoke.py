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


for r in results:
    print(json.dumps(r))
sys.exit(0 if results and all(r["pass"] for r in results) else 1)
