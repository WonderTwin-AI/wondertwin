"""SDK smoke suite: runs the PostHog app emulator's capture flows through the
official posthog-python SDK (posthog 7.91.1, pinned by uv.lock) against a
running binary.

    POSTHOG_EMULATOR_URL=http://localhost:4114 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails. Case names match go/smoke.go so run.sh can put both
SDKs in one table. Each case runs on a freshly reset emulator and reads what the
SDK sent back through /admin.
"""

import json
import os
import sys

import requests
from posthog import Posthog

BASE = os.environ.get("POSTHOG_EMULATOR_URL", "http://localhost:4114").rstrip("/")
SDK = "posthog-python 7.91.1"
KEY = "phc_sdksmoke000000000000000000000000000000"

results = []


def admin(method, path):
    resp = requests.request(method, BASE + path, timeout=10)
    resp.raise_for_status()
    return resp.json() if resp.content else None


def client(errors):
    # sync_mode sends each call before it returns, so a failure surfaces here.
    return Posthog(KEY, host=BASE, sync_mode=True, enable_local_evaluation=False,
                   on_error=lambda e, batch: errors.append(str(e)))


def case(name):
    def wrap(fn):
        errors = []
        try:
            admin("POST", "/admin/reset")
            c = client(errors)
            fn(c)
            c.shutdown()
            if errors:
                raise RuntimeError(errors[0])
            results.append({"sdk": SDK, "case": name, "pass": True, "detail": ""})
        except Exception as e:  # noqa: BLE001 - every failure becomes a row
            results.append({"sdk": SDK, "case": name, "pass": False, "detail": f"{type(e).__name__}: {e}"})
        return fn
    return wrap


@case("capture one event")
def _(c):
    c.capture("page_view", distinct_id="u1")
    got = admin("GET", "/admin/events?event=page_view")["events"]
    assert len(got) == 1 and got[0]["distinct_id"] == "u1", got


@case("capture with properties")
def _(c):
    c.capture("upgraded", distinct_id="u2", properties={"plan": "pro", "seats": 3})
    got = admin("GET", "/admin/events?event=upgraded")["events"]
    assert len(got) == 1 and got[0]["properties"].get("plan") == "pro", got


@case("capture several events")
def _(c):
    for name in ("a", "b", "c"):
        c.capture("evt_" + name, distinct_id="u3")
    got = admin("GET", "/admin/events?distinct_id=u3")["events"]
    assert len(got) == 3, got


@case("identify stores person properties")
def _(c):
    c.capture("$identify", distinct_id="u4", properties={"$set": {"email": "u4@example.com"}})
    got = admin("GET", "/admin/persons?distinct_id=u4")["persons"]
    assert len(got) == 1 and got[0]["properties"].get("email") == "u4@example.com", got


@case("alias links two ids")
def _(c):
    c.alias("u5", "u5-alias")
    got = admin("GET", "/admin/aliases")["aliases"]
    assert len(got) == 1 and got[0]["alias"] == "u5-alias", got


for r in results:
    print(json.dumps(r))
sys.exit(0 if all(r["pass"] for r in results) else 1)
