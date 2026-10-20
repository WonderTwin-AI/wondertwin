"""SDK smoke suite: runs the Plaid app emulator's Sandbox Item flow through the
official plaid-python SDK against a running binary. The SDK deserializes every
response strictly, so a response that breaks Plaid's field-presence rules (an
optional field sent as null, a required field missing) fails here.

    PLAID_EMULATOR_URL=http://localhost:4196 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails.
"""

import json
import os
import sys

import plaid
from plaid.api import plaid_api
from plaid.model.country_code import CountryCode
from plaid.model.item_get_request import ItemGetRequest
from plaid.model.item_public_token_exchange_request import ItemPublicTokenExchangeRequest
from plaid.model.link_token_create_request import LinkTokenCreateRequest
from plaid.model.link_token_create_request_user import LinkTokenCreateRequestUser
from plaid.model.link_token_get_request import LinkTokenGetRequest
from plaid.model.products import Products
from plaid.model.sandbox_public_token_create_request import SandboxPublicTokenCreateRequest

BASE = os.environ.get("PLAID_EMULATOR_URL", "http://localhost:4196").rstrip("/")
SDK = "plaid-python 44.0.0"

client = plaid_api.PlaidApi(plaid.ApiClient(plaid.Configuration(
    host=BASE, api_key={"clientId": "sdk-smoke", "secret": "secret"})))


def public_token(**kwargs):
    req = SandboxPublicTokenCreateRequest(institution_id="ins_109508", initial_products=[Products("auth")], **kwargs)
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
