"""SDK smoke suite: runs each MVP use case of the Slack app emulator through
the official slack-sdk against a running binary.

    SLACK_EMULATOR_URL=http://localhost:4197 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails. run.sh drives both SDK suites.
"""

import json
import os
import sys
import time

from slack_sdk import WebClient
from slack_sdk.version import __version__

BASE = os.environ.get("SLACK_EMULATOR_URL", "http://localhost:4197").rstrip("/")
TOKEN = "xoxb-sdk-smoke-python"
SDK = f"slack-sdk {__version__}"

client = WebClient(token=TOKEN, base_url=f"{BASE}/api/")
_counter = 0


def new_channel():
    global _counter
    _counter += 1
    res = client.conversations_create(name=f"smoke-{int(time.time() * 1000):x}{_counter}")
    assert res["ok"] is True
    return res["channel"]["id"]


def case_post_and_thread():
    assert client.auth_test()["ok"] is True
    channel = new_channel()
    parent = client.chat_postMessage(channel=channel, text="build finished")
    assert parent["ok"] is True and parent["ts"]
    reply = client.chat_postMessage(channel=channel, text="logs attached", thread_ts=parent["ts"])
    assert reply["ts"]
    thread = client.conversations_replies(channel=channel, ts=parent["ts"])
    in_thread = [m for m in thread["messages"] if m["ts"] == reply["ts"]]
    assert in_thread, "the reply appears in conversations.replies for the parent ts"
    assert in_thread[0]["thread_ts"] == parent["ts"]


def case_message_lifecycle():
    channel = new_channel()
    posted = client.chat_postMessage(channel=channel, text="draft")
    client.chat_update(channel=channel, ts=posted["ts"], text="final")
    history = client.conversations_history(channel=channel)
    assert [m for m in history["messages"] if m["ts"] == posted["ts"]][0]["text"] == "final"
    link = client.chat_getPermalink(channel=channel, message_ts=posted["ts"])
    assert link["permalink"]
    client.chat_delete(channel=channel, ts=posted["ts"])
    history = client.conversations_history(channel=channel)
    assert not [m for m in history["messages"] if m["ts"] == posted["ts"]]


CASES = {
    "slack-bot-post-and-thread": case_post_and_thread,
    "slack-message-lifecycle": case_message_lifecycle,
}


def main():
    failed = 0
    for name, fn in CASES.items():
        detail = ""
        try:
            fn()
            ok = True
        except Exception as err:  # noqa: BLE001 - every failure is a failed case
            ok = False
            failed += 1
            detail = f"{type(err).__name__}: {err}".splitlines()[0]
        print(json.dumps({"sdk": SDK, "case": name, "pass": ok, "detail": detail}), flush=True)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
