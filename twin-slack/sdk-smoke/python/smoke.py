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
from slack_sdk.errors import SlackApiError
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


def case_read_history_paginated():
    channel = new_channel()
    for i in range(1, 6):
        client.chat_postMessage(channel=channel, text=f"m{i}")
    # Iterating the response follows response_metadata.next_cursor to the end.
    found, pages = [], 0
    for page in client.conversations_history(channel=channel, limit=2):
        pages += 1
        found += [m["text"] for m in page["messages"]]
    assert found == ["m5", "m4", "m3", "m2", "m1"], found
    assert pages == 3, pages


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


def platform_error(fn):
    try:
        fn()
    except SlackApiError as err:
        return err.response["error"]
    raise AssertionError("expected the call to fail")


def case_error_unknown_method():
    # channels.list was retired by Slack in 2021. Slack answers it, and a name
    # that never existed, with unknown_method.
    assert platform_error(lambda: client.api_call("channels.list")) == "unknown_method"
    assert platform_error(lambda: client.api_call("definitely.notAMethod")) == "unknown_method"


def case_error_channel_not_found():
    assert platform_error(lambda: client.chat_postMessage(channel="C0NOSUCH", text="x")) == "channel_not_found"


def anonymous_client():
    return WebClient(token=None, base_url=f"{BASE}/api/")


def case_auth_test_reflects_token():
    bot = client.auth_test()
    assert bot.get("bot_id"), "a bot token carries a bot_id"
    user = WebClient(token="xoxp-sdk-smoke-python-user", base_url=f"{BASE}/api/").auth_test()
    assert not user.get("bot_id"), "a user token has no bot_id"
    assert user["user_id"] != bot["user_id"]
    assert user["team_id"] == bot["team_id"]


def case_api_test():
    res = anonymous_client().api_test(foo="bar")
    assert res["ok"] is True and res["args"]["foo"] == "bar"
    assert platform_error(lambda: client.api_test(error="my_error")) == "my_error"


def case_auth_revoke():
    doomed = WebClient(token=f"xoxb-sdk-smoke-python-revoke-{int(time.time() * 1000):x}", base_url=f"{BASE}/api/")
    assert doomed.auth_revoke()["revoked"] is True
    assert platform_error(lambda: doomed.auth_test()) == "token_revoked"
    assert client.auth_test()["ok"] is True


def case_error_not_authed():
    assert platform_error(lambda: anonymous_client().auth_test()) == "not_authed"


def case_error_invalid_auth():
    bad = WebClient(token="not-a-slack-token", base_url=f"{BASE}/api/")
    assert platform_error(lambda: bad.auth_test()) == "invalid_auth"


CASES = {
    "slack-bot-post-and-thread": case_post_and_thread,
    "slack-read-history-paginated": case_read_history_paginated,
    "slack-message-lifecycle": case_message_lifecycle,
    "error-unknown-method": case_error_unknown_method,
    "error-channel-not-found": case_error_channel_not_found,
    "slack-auth-test-reflects-token": case_auth_test_reflects_token,
    "slack-api-test": case_api_test,
    "slack-auth-revoke": case_auth_revoke,
    "error-not-authed": case_error_not_authed,
    "error-invalid-auth": case_error_invalid_auth,
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
