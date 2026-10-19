"""SDK smoke suite: runs the Xero app emulator's token and connection flows
through the official xero-python SDK against a running binary.

    XERO_EMULATOR_URL=http://localhost:4195 uv run smoke.py

The SDK hard-codes https://identity.xero.com in TokenApi, so the suite points
its URL attributes at the emulator. The SDK has no authorization-code helper
(the sample app does that with a third-party OAuth library), so the first
refresh token comes from a plain form POST. Prints one JSON line per case
({"sdk","case","pass","detail"}) and exits non-zero if any case fails.
"""

import http.client
import json
import os
import sys
import time
import urllib.parse

from xero_python.api_client import ApiClient, Configuration
from xero_python.api_client.oauth2 import OAuth2Token, TokenApi
from xero_python.accounting import AccountingApi
from xero_python.identity import IdentityApi

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
    conn = http.client.HTTPConnection(urllib.parse.urlsplit(BASE).netloc)
    conn.request("POST", "/connect/token", body, {"Content-Type": "application/x-www-form-urlencoded"})
    res = conn.getresponse()
    assert res.status == 200, f"code exchange failed: {res.status}"
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


def set_strict_auth(on):
    conn = http.client.HTTPConnection(urllib.parse.urlsplit(BASE).netloc)
    conn.request("PUT", "/admin/config", json.dumps({"strict_auth": on}), {"Content-Type": "application/json"})
    assert conn.getresponse().status == 200, "PUT /admin/config failed"


def made_up_client():
    """What localstack/finance-ops seeds: a token the emulator never issued."""
    client, token = new_client()
    seeded = {"access_token": "xero-local-test-token", "refresh_token": "xero-local-test-refresh", "token_type": "Bearer",
              "scope": "accounting.contacts offline_access", "expires_in": 1800}
    token.update_token(**seeded, expires_at=time.time() + 1800)
    client.set_oauth2_token(seeded)
    return client, token


def case_made_up_tokens_are_accepted_by_default():
    client, token = made_up_client()
    assert len(IdentityApi(client, base_url=BASE).get_connections()) == 1
    token.expires_at = time.time() - 1  # the SDK now refreshes with the made-up refresh token
    assert token.get_valid_access_token(client) != "xero-local-test-token"


def case_strict_auth_refuses_made_up_tokens():
    set_strict_auth(True)
    try:
        client, token = made_up_client()
        try:
            IdentityApi(client, base_url=BASE).get_connections()
            raise AssertionError("strict auth let a made-up bearer token through")
        except AssertionError:
            raise
        except Exception as e:  # the SDK raises on a 401
            assert "401" in str(e) or "nauthorized" in str(e), str(e)
        try:
            token.refresh_access_token(client)
            raise AssertionError("strict auth let a made-up refresh token through")
        except AssertionError:
            raise
        except Exception as e:
            assert "invalid_grant" in str(e), str(e)
    finally:
        set_strict_auth(False)


def case_connect_tenant():
    """The xero-connect-tenant flow: exchange a code, list the connected tenants
    (GET /Connections, zoneless ISO dates), then read the organisation with the
    tenant id from the connection."""
    client, _, _ = signed_in()
    connections = IdentityApi(client, base_url=BASE).get_connections()
    assert len(connections) == 1, f"expected the one default tenant, got {len(connections)}"
    c = connections[0]
    assert c.tenant_type == "ORGANISATION" and c.tenant_id and c.id and c.auth_event_id
    assert c.created_date_utc.year >= 2026 and c.created_date_utc.tzinfo is not None
    # The tenant id comes from the connection, as it does for a real client (auth-001).
    orgs = AccountingApi(client, base_url=f"{BASE}/api.xro/2.0").get_organisations(c.tenant_id).organisations
    assert len(orgs) == 1 and orgs[0].name and orgs[0].version == "GLOBAL"
    assert orgs[0].created_date_utc.year >= 2026, "the SDK parsed CreatedDateUTC as a /Date()/ value"


CASES = [
    ("made-up tokens are accepted by default", case_made_up_tokens_are_accepted_by_default),
    ("strict auth refuses made-up tokens", case_strict_auth_refuses_made_up_tokens),
    ("connect: a code exchange, then get_connections returns the tenant", case_connect_tenant),
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
