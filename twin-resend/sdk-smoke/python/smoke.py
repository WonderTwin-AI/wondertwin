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


for r in results:
    print(json.dumps(r))
sys.exit(0 if results and all(r["pass"] for r in results) else 1)
