"""SDK smoke suite: runs each MVP use case of the Slack app emulator through
the official slack-sdk against a running binary.

    SLACK_EMULATOR_URL=http://localhost:4197 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails. run.sh drives both SDK suites.
"""

import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from http.client import HTTPConnection
from urllib.parse import urlsplit

from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError
from slack_sdk.signature import SignatureVerifier
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
    # Find the channel the way an app does: page through conversations.list,
    # which lists public channels and no direct messages by default. A DM
    # exists, so the default list has one to leave out.
    client.conversations_open(users=client.auth_test()["user_id"])
    listed = [c for page in client.conversations_list(limit=200) for c in page["channels"]]
    assert any(c["id"] == channel for c in listed), "conversations.list finds the channel"
    assert all(not c.get("is_im") and not c.get("is_mpim") for c in listed), "no direct messages in the default list"
    for i in range(1, 6):
        client.chat_postMessage(channel=channel, text=f"m{i}")
    # Iterating the response follows response_metadata.next_cursor to the end.
    found, pages = [], 0
    for page in client.conversations_history(channel=channel, limit=2):
        pages += 1
        found += [m["text"] for m in page["messages"]]
    assert found == ["m5", "m4", "m3", "m2", "m1"], found
    assert pages == 3, pages


def case_oauth_v2_install():
    # The install's last step: the app, which has no token yet, exchanges the
    # code from the authorize redirect, then uses the bot token it got. The
    # SDK sends the client credentials by HTTP Basic.
    code = f"code-{int(time.time() * 1000):x}"
    install = anonymous_client().oauth_v2_access(client_id="123.456", client_secret="smoke-secret", code=code)
    assert install["ok"] is True
    assert install["token_type"] == "bot"
    assert install["access_token"].startswith("xoxb-"), install["access_token"]
    assert install["scope"], "the granted scopes are returned"
    who = WebClient(token=install["access_token"], base_url=f"{BASE}/api/").auth_test()
    assert who["ok"] is True
    assert who["team_id"] == install["team"]["id"]


def case_upload_file_external():
    # files_upload_v2 drives the three-step external upload: get an upload URL,
    # POST the bytes to it, then complete the upload into a channel.
    channel = new_channel()
    content = b"release notes for the smoke test\n"
    res = client.files_upload_v2(channel=channel, content=content, filename="notes.txt", title="Notes")
    assert res["ok"] is True
    file_id = res["files"][0]["id"]
    info = client.files_info(file=file_id)
    assert info["file"]["id"] == file_id
    assert channel in info["file"]["channels"], info["file"].get("channels")
    assert info["file"]["size"] == len(content), info["file"].get("size")


def case_app_home_publish():
    # The app publishes a Home tab for the user who opened it. The SDK sends a
    # JSON body with the view as an object.
    user_id = WebClient(token="xoxp-sdk-smoke-python-home", base_url=f"{BASE}/api/").auth_test()["user_id"]
    external_id = f"home-{int(time.time() * 1000):x}"
    view = {"type": "home", "external_id": external_id, "blocks": [{"type": "section", "text": {"type": "mrkdwn", "text": "Welcome"}}]}
    first = client.views_publish(user_id=user_id, view=view)
    assert first["ok"] is True
    assert first["view"]["id"], "the published view has an id"
    assert first["view"]["type"] == "home"
    again = client.views_publish(user_id=user_id, view=view, hash=first["view"]["hash"])
    assert again["view"]["id"] == first["view"]["id"], "a user has one Home view"
    updated = client.views_update(external_id=external_id, view={**view, "callback_id": "home_v2"})
    assert updated["view"]["id"] == first["view"]["id"]
    assert updated["view"]["callback_id"] == "home_v2"
    assert platform_error(lambda: client.views_publish(user_id=user_id, view=view, hash=first["view"]["hash"])) == "hash_conflict"


def case_dm_user():
    # Fixture: a workspace user to find. Seeding is the emulator's admin API,
    # not the SDK's; everything after it goes through the SDK.
    email = f"dm-{int(time.time() * 1000):x}@example.com"
    users = {"U_SMOKE_PY": {"id": "U_SMOKE_PY", "name": "smoke-py", "profile": {"email": email}}}
    conn = HTTPConnection(urlsplit(BASE).netloc)
    conn.request("POST", "/admin/state", json.dumps({"users": users}), {"Content-Type": "application/json"})
    resp = conn.getresponse()
    resp.read()
    conn.close()
    assert resp.status == 200
    found = client.users_lookupByEmail(email=email)
    opened = client.conversations_open(users=found["user"]["id"])
    assert opened["ok"] is True
    channel = opened["channel"]["id"]
    again = client.conversations_open(users=found["user"]["id"])
    assert again["channel"]["id"] == channel, "opening again resumes the same DM"
    assert again["already_open"] is True
    posted = client.chat_postMessage(channel=channel, text="hello there")
    history = client.conversations_history(channel=channel)
    assert any(m["ts"] == posted["ts"] and m["text"] == "hello there" for m in history["messages"])


def case_react_pin_bookmark():
    channel = new_channel()
    ts = client.chat_postMessage(channel=channel, text="ship it")["ts"]
    client.reactions_add(channel=channel, timestamp=ts, name="rocket")
    got = client.reactions_get(channel=channel, timestamp=ts)
    assert any(r["name"] == "rocket" and r["count"] == 1 for r in got["message"]["reactions"])
    client.pins_add(channel=channel, timestamp=ts)
    assert platform_error(lambda: client.pins_add(channel=channel, timestamp=ts)) == "already_pinned"
    pins = client.pins_list(channel=channel)
    assert len([i for i in pins["items"] if i["message"]["ts"] == ts]) == 1, "the message is pinned once"
    assert client.bookmarks_list(channel_id=channel)["bookmarks"] == []
    client.bookmarks_add(channel_id=channel, title="Runbook", type="link", link="https://example.com/runbook")
    bookmarks = client.bookmarks_list(channel_id=channel)
    assert any(b["link"] == "https://example.com/runbook" for b in bookmarks["bookmarks"])


def case_events_http_receive():
    # A local receiver stands in for the app's Request URL. It fails the first
    # delivery, so the event arrives a second time as retry 1. The official
    # SignatureVerifier checks every delivery.
    got = []

    class Receiver(BaseHTTPRequestHandler):
        def do_POST(self):
            body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
            got.append((dict(self.headers), body))
            self.send_response(500 if len(got) == 1 else 200)
            self.end_headers()

        def log_message(self, *args):
            pass

    receiver = HTTPServer(("127.0.0.1", 0), Receiver)
    threading.Thread(target=receiver.serve_forever, daemon=True).start()
    secret = f"smoke-python-{int(time.time() * 1000):x}"

    def configure(cfg):
        conn = HTTPConnection(urlsplit(BASE).netloc)
        conn.request("POST", "/admin/events/config", json.dumps(cfg), {"Content-Type": "application/json"})
        resp = conn.getresponse()
        resp.read()
        conn.close()
        assert resp.status == 200

    try:
        configure({"request_url": f"http://127.0.0.1:{receiver.server_port}/slack/events",
                   "signing_secret": secret, "retry_delays_ms": [0, 0, 0]})
        channel = new_channel()
        posted = client.chat_postMessage(channel=channel, text="deploy started")
        for _ in range(200):
            if len(got) >= 2:
                break
            time.sleep(0.025)
        assert len(got) == 2, f"the failed delivery is retried once and then succeeds, got {len(got)}"
        verifier = SignatureVerifier(signing_secret=secret)
        for headers, body in got:
            assert verifier.is_valid_request(body, headers), "SignatureVerifier rejected the delivery"
        lower = [{k.lower(): v for k, v in h.items()} for h, _ in got]
        assert "x-slack-retry-num" not in lower[0]
        assert lower[1]["x-slack-retry-num"] == "1"
        assert lower[1]["x-slack-retry-reason"] == "http_error"
        env = json.loads(got[1][1])
        assert env["type"] == "event_callback"
        assert env["event"]["type"] == "message"
        assert env["event"]["channel"] == channel
        assert env["event"]["ts"] == posted["ts"]
    finally:
        configure({"request_url": ""})
        receiver.shutdown()


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
    "slack-oauth-v2-install": case_oauth_v2_install,
    "slack-upload-file-external": case_upload_file_external,
    "slack-app-home-publish": case_app_home_publish,
    "slack-dm-user": case_dm_user,
    "slack-react-pin-bookmark": case_react_pin_bookmark,
    "slack-events-http-receive": case_events_http_receive,
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
