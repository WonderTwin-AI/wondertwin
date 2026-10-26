"""SDK smoke suite: runs the Plaid app emulator's Sandbox Item flow through the
official plaid-python SDK against a running binary. The SDK deserializes every
response strictly, so a response that breaks Plaid's field-presence rules (an
optional field sent as null, a required field missing) fails here.

    PLAID_EMULATOR_URL=http://localhost:4196 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails.
"""

import base64
import hashlib
import http.server
import json
import os
import queue
import sys
import threading

import plaid
from plaid.api import plaid_api
from plaid.model.accounts_balance_get_request import AccountsBalanceGetRequest
from plaid.model.accounts_balance_get_request_options import AccountsBalanceGetRequestOptions
from plaid.model.accounts_get_request import AccountsGetRequest
from plaid.model.country_code import CountryCode
from plaid.model.institutions_get_by_id_request import InstitutionsGetByIdRequest
from plaid.model.institutions_get_request import InstitutionsGetRequest
from plaid.model.institutions_search_request import InstitutionsSearchRequest
from plaid.model.item_access_token_invalidate_request import ItemAccessTokenInvalidateRequest
from plaid.model.item_get_request import ItemGetRequest
from plaid.model.item_remove_request import ItemRemoveRequest
from plaid.model.item_webhook_update_request import ItemWebhookUpdateRequest
from plaid.model.item_public_token_exchange_request import ItemPublicTokenExchangeRequest
from plaid.model.link_token_create_request import LinkTokenCreateRequest
from plaid.model.link_token_create_request_user import LinkTokenCreateRequestUser
from plaid.model.link_token_get_request import LinkTokenGetRequest
from plaid.model.products import Products
from plaid.model.webhook_verification_key_get_request import WebhookVerificationKeyGetRequest
from plaid.model.sandbox_item_fire_webhook_request import SandboxItemFireWebhookRequest
from plaid.model.sandbox_item_reset_login_request import SandboxItemResetLoginRequest
from plaid.model.sandbox_public_token_create_request import SandboxPublicTokenCreateRequest
from plaid.model.sandbox_public_token_create_request_options import SandboxPublicTokenCreateRequestOptions

BASE = os.environ.get("PLAID_EMULATOR_URL", "http://localhost:4196").rstrip("/")
SDK = "plaid-python 44.0.0"

client = plaid_api.PlaidApi(plaid.ApiClient(plaid.Configuration(
    host=BASE, api_key={"clientId": "sdk-smoke", "secret": "secret"})))


def public_token(products=("auth",), **kwargs):
    req = SandboxPublicTokenCreateRequest(
        institution_id="ins_109508", initial_products=[Products(p) for p in products], **kwargs)
    return client.sandbox_public_token_create(req)


def link_token(**kwargs):
    return client.link_token_create(LinkTokenCreateRequest(
        client_name="Acme", language="en", country_codes=[CountryCode("US")], products=[Products("auth")],
        user=LinkTokenCreateRequestUser(client_user_id="smoke-user"), **kwargs))


def case_link_token_create():
    res = link_token()
    assert res.link_token.startswith("link-sandbox-") and res.expiration and res.request_id


def case_link_token_get():
    created = link_token(webhook="http://hooks.test/plaid")
    got = client.link_token_get(LinkTokenGetRequest(link_token=created.link_token))
    assert got.link_token == created.link_token and got.expiration == created.expiration
    assert got.metadata.client_name == "Acme" and got.metadata.webhook == "http://hooks.test/plaid"
    assert got.metadata.redirect_uri is None and [str(p) for p in got.metadata.initial_products] == ["auth"]


def case_link_token_then_exchange():
    link_token()
    assert client.item_public_token_exchange(
        ItemPublicTokenExchangeRequest(public_token=public_token().public_token)).access_token


def case_sandbox_public_token_create():
    res = public_token()
    assert res.public_token.startswith("public-sandbox-") and res.request_id


def case_item_public_token_exchange():
    res = client.item_public_token_exchange(ItemPublicTokenExchangeRequest(public_token=public_token().public_token))
    assert res.access_token.startswith("access-sandbox-") and len(res.item_id) == 37


def case_accounts_get():
    exchanged = client.item_public_token_exchange(ItemPublicTokenExchangeRequest(public_token=public_token().public_token))
    res = client.accounts_get(AccountsGetRequest(access_token=exchanged.access_token))
    assert res.item.item_id == exchanged.item_id
    assert [a.name for a in res.accounts] == ["Plaid Checking", "Plaid 401k", "Plaid Student Loan"]
    for account in res.accounts:
        assert "persistent_account_id" not in account.to_dict(), "persistent_account_id must be absent, never null"
    checking = res.accounts[0]
    assert str(checking.type) == "depository" and checking.balances.available == 100 and checking.balances.limit is None


def linked_access_token(products=("auth",), **kwargs):
    return client.item_public_token_exchange(
        ItemPublicTokenExchangeRequest(public_token=public_token(products, **kwargs).public_token)).access_token


def api_error(call):
    try:
        call()
    except plaid.ApiException as e:
        return e.status, json.loads(e.body)["error_code"]
    raise AssertionError("the call was accepted")


def case_item_remove_then_item_get_is_item_not_found():
    token = linked_access_token()
    assert client.item_remove(ItemRemoveRequest(access_token=token)).request_id
    assert api_error(lambda: client.item_get(ItemGetRequest(access_token=token))) == (400, "ITEM_NOT_FOUND")


def case_item_access_token_invalidate_issues_a_new_token():
    old = linked_access_token()
    new = client.item_access_token_invalidate(ItemAccessTokenInvalidateRequest(access_token=old)).new_access_token
    assert new.startswith("access-sandbox-") and new != old
    assert api_error(lambda: client.item_get(ItemGetRequest(access_token=old))) == (400, "INVALID_ACCESS_TOKEN")
    assert client.item_get(ItemGetRequest(access_token=new)).item.item_id


def case_error_password_fails_the_create():
    options = SandboxPublicTokenCreateRequestOptions(override_username="user_good", override_password="error_ITEM_LOCKED")
    assert api_error(lambda: public_token(options=options)) == (400, "ITEM_LOCKED")


def case_reset_login_puts_the_item_in_login_required():
    token = linked_access_token()
    assert client.sandbox_item_reset_login(SandboxItemResetLoginRequest(access_token=token)).reset_login is True
    assert client.item_get(ItemGetRequest(access_token=token)).item.error.error_code == "ITEM_LOGIN_REQUIRED"
    assert api_error(lambda: client.accounts_get(AccountsGetRequest(access_token=token))) == (400, "ITEM_LOGIN_REQUIRED")
    assert link_token(access_token=token).link_token.startswith("link-sandbox-"), "update mode must still work"


def case_webhook_verification_key_get_refuses_an_unknown_key_id():
    # A client learns a real key id only from a delivered webhook's JWT header, so the
    # positive path is checked once delivery exists; the sign and verify round trip is a Go test.
    assert api_error(lambda: client.webhook_verification_key_get(
        WebhookVerificationKeyGetRequest(key_id="not-a-key-id"))) == (400, "INVALID_WEBHOOK_VERIFICATION_KEY_ID")
US = [CountryCode("US")]


def case_institutions_get_pages_by_offset():
    first = client.institutions_get(InstitutionsGetRequest(count=5, offset=0, country_codes=US))
    second = client.institutions_get(InstitutionsGetRequest(count=5, offset=5, country_codes=US))
    assert len(first.institutions) == 5 and first.total == second.total and first.total > 10
    assert not {i.institution_id for i in first.institutions} & {i.institution_id for i in second.institutions}


def case_institutions_search():
    res = client.institutions_search(InstitutionsSearchRequest(query="houndstooth", country_codes=US, products=[Products("auth")]))
    assert [i.name for i in res.institutions] == ["Houndstooth Bank"]


def case_institutions_get_by_id():
    inst = client.institutions_get_by_id(InstitutionsGetByIdRequest(institution_id="ins_109512", country_codes=US)).institution
    assert inst.name == "Houndstooth Bank" and inst.oauth is False and len(inst.routing_numbers) == 3
    assert api_error(lambda: client.institutions_get_by_id(
        InstitutionsGetByIdRequest(institution_id="ins_nope", country_codes=US))) == (400, "INVALID_INSTITUTION")


def start_receiver():
    """A webhook URL on localhost; every POST it gets lands on the returned queue."""
    got = queue.Queue()

    class Hook(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            got.put((self.rfile.read(int(self.headers.get("Content-Length", 0))), dict(self.headers)))
            self.send_response(200)
            self.end_headers()

        def log_message(self, *args):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Hook)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return f"http://127.0.0.1:{server.server_port}/hook", got


def case_item_webhook_update_delivers_a_signed_flat_body():
    url, got = start_receiver()
    token = linked_access_token()
    assert client.item_webhook_update(ItemWebhookUpdateRequest(access_token=token, webhook=url)).item.webhook == url
    body, headers = got.get(timeout=5)
    payload = json.loads(body)
    assert (payload["webhook_type"], payload["webhook_code"], payload["new_webhook_url"]) == ("ITEM", "WEBHOOK_UPDATE_ACKNOWLEDGED", url)
    assert "data" not in payload and payload["error"] is None, "the body must be flat, not wrapped in an envelope"
    head, claims, _ = headers["Plaid-Verification"].split(".")
    assert json.loads(unb64(claims))["request_body_sha256"] == hashlib.sha256(body).hexdigest()
    # A delivered webhook is how a client learns a key id; the SDK can then fetch that key.
    key = client.webhook_verification_key_get(WebhookVerificationKeyGetRequest(key_id=json.loads(unb64(head))["kid"])).key
    assert key.alg == "ES256" and key.crv == "P-256" and key.expired_at is None


# ES256 verification with the standard library, so the harness needs no crypto dependency.
P = 0xFFFFFFFF00000001000000000000000000000000FFFFFFFFFFFFFFFFFFFFFFFF
N = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
G = (0x6B17D1F2E12C4247F8BCE6E563A440F277037D812DEB33A0F4A13945D898C296,
     0x4FE342E2FE1A7F9B8EE7EB4A7C0F9E162BCE33576B315ECECBB6406837BF51F5)


def _add(a, b):
    if a is None or b is None:
        return a or b
    if a[0] == b[0] and (a[1] + b[1]) % P == 0:
        return None
    m = ((3 * a[0] * a[0] - 3) * pow(2 * a[1], -1, P) if a == b else (b[1] - a[1]) * pow(b[0] - a[0], -1, P)) % P
    x = (m * m - a[0] - b[0]) % P
    return x, (m * (a[0] - x) - a[1]) % P


def _mul(k, pt):
    out = None
    while k:
        if k & 1:
            out = _add(out, pt)
        pt, k = _add(pt, pt), k >> 1
    return out


def es256_verify(key, signing_input, sig):
    unb = lambda v: int.from_bytes(base64.urlsafe_b64decode(v + "=" * (-len(v) % 4)), "big")
    r, s_ = int.from_bytes(sig[:32], "big"), int.from_bytes(sig[32:], "big")
    e, w = int.from_bytes(hashlib.sha256(signing_input).digest(), "big"), pow(s_, -1, N)
    pt = _add(_mul(e * w % N, G), _mul(r * w % N, (unb(key.x), unb(key.y))))
    return pt is not None and pt[0] % N == r


def unb64(part):
    return base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))


def case_fire_webhook_delivers_a_body_the_served_key_verifies():
    url, got = start_receiver()
    token = linked_access_token(("auth", "transactions"), options=SandboxPublicTokenCreateRequestOptions(webhook=url))
    assert client.sandbox_item_fire_webhook(SandboxItemFireWebhookRequest(
        access_token=token, webhook_code="SYNC_UPDATES_AVAILABLE")).webhook_fired is True
    body, headers = got.get(timeout=5)
    payload = json.loads(body)
    assert (payload["webhook_type"], payload["webhook_code"]) == ("TRANSACTIONS", "SYNC_UPDATES_AVAILABLE") and "data" not in payload
    head, claims, sig = headers["Plaid-Verification"].split(".")
    assert json.loads(unb64(claims))["request_body_sha256"] == hashlib.sha256(body).hexdigest()
    key = client.webhook_verification_key_get(WebhookVerificationKeyGetRequest(key_id=json.loads(unb64(head))["kid"])).key
    assert es256_verify(key, f"{head}.{claims}".encode(), unb64(sig)), "the served key does not verify the signature"
    assert not es256_verify(key, f"{head}.{claims}x".encode(), unb64(sig)), "the signature verified over other content"


def case_login_repair_flow_fires_a_webhook_and_update_mode_still_works():
    url, got = start_receiver()
    token = linked_access_token(options=SandboxPublicTokenCreateRequestOptions(webhook=url))
    assert client.sandbox_item_reset_login(SandboxItemResetLoginRequest(access_token=token)).reset_login is True
    payload = json.loads(got.get(timeout=5)[0])
    assert (payload["webhook_type"], payload["webhook_code"], payload["error"]["error_code"]) == ("ITEM", "ERROR", "ITEM_LOGIN_REQUIRED")
    assert api_error(lambda: client.accounts_get(AccountsGetRequest(access_token=token))) == (400, "ITEM_LOGIN_REQUIRED")
    assert link_token(access_token=token).link_token.startswith("link-sandbox-")
    client.sandbox_item_fire_webhook(SandboxItemFireWebhookRequest(access_token=token, webhook_code="LOGIN_REPAIRED"))
    assert json.loads(got.get(timeout=5)[0])["webhook_code"] == "LOGIN_REPAIRED"


def case_accounts_balance_get_and_its_account_ids_filter():
    token = linked_access_token()
    res = client.accounts_balance_get(AccountsBalanceGetRequest(access_token=token))
    assert [a.name for a in res.accounts] == ["Plaid Checking", "Plaid 401k", "Plaid Student Loan"]
    assert res.accounts[0].balances.available == 100 and res.accounts[0].balances.current == 110
    savings = res.accounts[2].account_id
    only = client.accounts_balance_get(AccountsBalanceGetRequest(
        access_token=token, options=AccountsBalanceGetRequestOptions(account_ids=[savings]))).accounts
    assert [a.account_id for a in only] == [savings]
    assert api_error(lambda: client.accounts_balance_get(AccountsBalanceGetRequest(
        access_token=token, options=AccountsBalanceGetRequestOptions(account_ids=["nope"])))) == (400, "INVALID_ACCOUNT_ID")


def case_item_get():
    exchanged = client.item_public_token_exchange(ItemPublicTokenExchangeRequest(public_token=public_token().public_token))
    item = client.item_get(ItemGetRequest(access_token=exchanged.access_token)).item
    assert item.item_id == exchanged.item_id and item.institution_id == "ins_109508"
    assert item.webhook is None and item.error is None and item.update_type == "background"


def case_public_token_is_single_use():
    token = public_token().public_token
    client.item_public_token_exchange(ItemPublicTokenExchangeRequest(public_token=token))
    try:
        client.item_public_token_exchange(ItemPublicTokenExchangeRequest(public_token=token))
    except plaid.ApiException as e:
        body = json.loads(e.body)
        assert e.status == 400 and body["error_code"] == "INVALID_PUBLIC_TOKEN", body
    else:
        raise AssertionError("a spent public token was exchanged twice")


def case_item_get_refuses_an_unknown_access_token():
    try:
        client.item_get(ItemGetRequest(access_token="access-sandbox-00000000-0000-0000-0000-000000000000"))
    except plaid.ApiException as e:
        assert e.status == 400 and json.loads(e.body)["error_code"] == "INVALID_ACCESS_TOKEN"
    else:
        raise AssertionError("an unknown access token was accepted")


def main():
    failed = False
    for name, fn in sorted((n, f) for n, f in globals().items() if n.startswith("case_") and callable(f)):
        try:
            fn()
            row = {"sdk": SDK, "case": name[5:], "pass": True, "detail": ""}
        except Exception as e:  # report every case, not just the first failure
            failed = True
            row = {"sdk": SDK, "case": name[5:], "pass": False, "detail": f"{type(e).__name__}: {e}"}
        print(json.dumps(row))
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
