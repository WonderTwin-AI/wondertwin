"""SDK smoke suite: runs the Xero app emulator's token flows through the
official xero-python SDK against a running binary.

    XERO_EMULATOR_URL=http://localhost:4195 uv run smoke.py

The SDK hard-codes https://identity.xero.com in TokenApi, so the suite points
its URL attributes at the emulator. The SDK has no authorization-code helper
(the sample app does that with a third-party OAuth library), so the first
refresh token comes from a plain form POST. Prints one JSON line per case
({"sdk","case","pass","detail"}) and exits non-zero if any case fails.
"""

import json
import os
import sys
import time
import urllib.parse
import urllib.request

from xero_python.api_client import ApiClient, Configuration
from xero_python.api_client.oauth2 import OAuth2Token, TokenApi

BASE = os.environ.get("XERO_EMULATOR_URL", "http://localhost:4195").rstrip("/")
SDK = "xero-python 15.2.0"
CLIENT_ID, CLIENT_SECRET = "sdk-smoke", "secret"

TokenApi.refresh_token_url = TokenApi.client_credentials_token_url = f"{BASE}/connect/token"


def new_client():
    holder = {"token": None}
    token = OAuth2Token(client_id=CLIENT_ID, client_secret=CLIENT_SECRET)
    client = ApiClient(
        Configuration(oauth2_token=token),
        oauth2_token_getter=lambda: holder["token"],
        oauth2_token_saver=lambda t: holder.update(token=t),
    )
    return client, token


def code_exchange():
    body = urllib.parse.urlencode({"grant_type": "authorization_code", "code": "smoke", "client_id": CLIENT_ID,
                                   "client_secret": CLIENT_SECRET}).encode()
    with urllib.request.urlopen(urllib.request.Request(f"{BASE}/connect/token", data=body)) as res:
        return json.load(res)


def signed_in():
    client, token = new_client()
    first = code_exchange()
    token.update_token(**first, expires_at=time.time() + first["expires_in"])
    client.set_oauth2_token(first)
    return client, token, first


def case_refresh_rotates_the_token():
    client, token, first = signed_in()
    assert token.refresh_access_token(client) is True
    assert token.refresh_token and token.refresh_token != first["refresh_token"], "a refresh token is single use, so it rotates"
    assert token.access_token != first["access_token"] and token.expires_in == 1800
    assert token.token_type == "Bearer" and "accounting.contacts" in str(token.scope)


def case_replayed_refresh_token_is_refused():
    client, token, first = signed_in()
    token.refresh_access_token(client)
    try:
        TokenApi(client, CLIENT_ID, CLIENT_SECRET).refresh_token(first["refresh_token"], ["openid"])
    except Exception as e:  # the SDK raises a bare Exception with the status
        assert "400" in str(e) and "invalid_grant" in str(e), str(e)
        return
    raise AssertionError("the SDK accepted a refresh token that was already redeemed")


def case_expired_token_refreshes_on_demand():
    client, token, first = signed_in()
    token.expires_at = time.time() - 1
    fresh = token.get_valid_access_token(client)
    assert fresh and fresh != first["access_token"], "an expired token is refreshed through the emulator"


def case_client_credentials():
    client, token = new_client()
    assert token.get_client_credentials_access_token(client, False) is True
    assert token.access_token.count(".") == 2 and token.expires_in == 1800 and token.refresh_token is None


CASES = [
    ("refresh rotates the token", case_refresh_rotates_the_token),
    ("a replayed refresh token is refused", case_replayed_refresh_token_is_refused),
    ("an expired token refreshes on demand", case_expired_token_refreshes_on_demand),
    ("client credentials", case_client_credentials),
]

failed = False
for name, fn in CASES:
    try:
        fn()
        row = {"sdk": SDK, "case": name, "pass": True, "detail": ""}
    except Exception as e:  # noqa: BLE001 - every failure is a result row
        failed, row = True, {"sdk": SDK, "case": name, "pass": False, "detail": f"{type(e).__name__}: {e}"}
    print(json.dumps(row), flush=True)
sys.exit(1 if failed else 0)
