"""SDK smoke suite: runs the Xero app emulator's token and connection flows
through the official xero-python SDK against a running binary.

    XERO_EMULATOR_URL=http://localhost:4195 uv run smoke.py

The SDK hard-codes https://identity.xero.com in TokenApi, so the suite points
its URL attributes at the emulator. The SDK has no authorization-code helper
(the sample app does that with a third-party OAuth library), so the first
refresh token comes from a plain form POST. Prints one JSON line per case
({"sdk","case","pass","detail"}) and exits non-zero if any case fails.
"""

import datetime
import http.client
import json
import os
import sys
import time
import urllib.parse
import uuid

from xero_python.api_client import ApiClient, Configuration
from xero_python.api_client.oauth2 import OAuth2Token, TokenApi
from xero_python.accounting import Address, AccountingApi, Contact, Contacts, CurrencyCode, Invoice, Invoices, LineItem
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


def accounting_for(client):
    tenant = IdentityApi(client, base_url=BASE).get_connections()[0].tenant_id
    return AccountingApi(client, base_url=f"{BASE}/api.xro/2.0"), tenant


def case_create_contacts():
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    contact = Contact(name="Smoke Customer", first_name="smoke@example.test", last_name="Customer (cus_smoke)",
                      account_number="cus_smoke", is_customer=True,
                      addresses=[Address(address_type="STREET", city="Zurich", country="CH")])
    created = api.create_contacts(tenant, Contacts(contacts=[contact])).contacts  # sends PUT /Contacts
    assert len(created) == 1 and created[0].contact_id, "the SDK parsed the created contact"
    assert created[0].contact_status == "ACTIVE" and created[0].account_number == "cus_smoke"
    assert created[0].updated_date_utc.year >= 2026, "the SDK parsed UpdatedDateUTC as a /Date()/ value"


def case_duplicate_contact_name_is_refused():
    """finance-ops retries a name collision by matching 'contact name' and 'unique' in the SDK's error."""
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    api.create_contacts(tenant, Contacts(contacts=[Contact(name="Smoke Dup", account_number="dup_1")]))
    try:
        api.create_contacts(tenant, Contacts(contacts=[Contact(name="smoke dup")]))
    except Exception as e:  # the SDK raises on a 400
        text = str(e).lower()
        assert "contact name" in text and "unique" in text, text[:300]
        return
    raise AssertionError("a second contact with the same name was accepted")


def case_get_contacts_as_finance_ops_calls_it():
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    made = api.create_contacts(tenant, Contacts(contacts=[Contact(name="Lookup Me", account_number="cus_lookup")])).contacts[0]
    # finance-ops: get_contacts(tenant, where='AccountNumber="cus_x"', page=1), with a single =
    found = api.get_contacts(tenant, where='AccountNumber="cus_lookup"', page=1)
    assert [c.contact_id for c in found.contacts] == [made.contact_id], "the single = where finds the contact"
    assert found.pagination.page == 1 and found.pagination.item_count == 1, "the SDK parsed the pagination object"
    assert [c.contact_id for c in api.get_contacts(tenant, where='AccountNumber=="cus_lookup"').contacts] == [made.contact_id]
    assert [c.contact_id for c in api.get_contacts(tenant, i_ds=[made.contact_id]).contacts] == [made.contact_id]
    assert api.get_contacts(tenant, where='AccountNumber="nobody"', page=1).contacts == []


def case_get_contacts_parses_with_and_without_page():
    """finance-ops passes page=1 to dodge an xero-python bug: an empty pagination
    field comes back as [] and raises ValueError. So without page the field must be
    omitted or null, never [], and with page=1 it must be the object the SDK parses."""
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    api.create_contacts(tenant, Contacts(contacts=[Contact(name="Paging A"), Contact(name="Paging B")]))
    plain = api.get_contacts(tenant)  # no page: must parse, and carry no pagination
    assert len(plain.contacts) >= 2 and plain.pagination is None, f"pagination without page = {plain.pagination!r}"
    paged = api.get_contacts(tenant, page=1, page_size=1)
    p = paged.pagination
    assert (p.page, p.page_size, len(paged.contacts)) == (1, 1, 1) and p.item_count >= 2 and p.page_count >= 2, p


def case_chart_of_accounts():
    client, token, _ = signed_in()
    tenant = IdentityApi(client, base_url=BASE).get_connections()[0].tenant_id
    accounts = AccountingApi(client, base_url=f"{BASE}/api.xro/2.0").get_accounts(tenant).accounts
    by_code = {a.code: a for a in accounts}
    for code in ("090", "200", "201", "404"):  # the accounts localstack/finance-ops posts to
        assert code in by_code, f"account {code} is missing from the chart"
    assert by_code["090"].type.value == "BANK", "the SDK parsed Type into its AccountType enum"
    assert by_code["200"].updated_date_utc.year >= 2026, "the SDK parsed UpdatedDateUTC as a /Date()/ value"


def accounting_api():
    """An AccountingApi signed in as a code exchange, and the tenant id from the connection."""
    client, _, _ = signed_in()
    tenant = IdentityApi(client, base_url=BASE).get_connections()[0].tenant_id
    return AccountingApi(client, base_url=f"{BASE}/api.xro/2.0"), tenant


def new_contact(api, tenant):
    name = f"Smoke {uuid.uuid4().hex[:8]}"
    return api.create_contacts(tenant, Contacts(contacts=[Contact(name=name)])).contacts[0].contact_id


def finance_ops_invoice(contact_id, **kw):
    """The shape localstack/finance-ops sends: fee and negative lines carrying only a LineAmount,
    and a client-side Total that Xero ignores."""
    return Invoice(
        type="ACCREC", contact=Contact(contact_id=contact_id), status="AUTHORISED", currency_code=CurrencyCode("USD"),
        date=datetime.date(2026, 9, 1), due_date=datetime.date(2026, 9, 30), total=999,
        line_items=[LineItem(description="Subscription fee", line_amount=25.5, account_code="201", tax_type="TAX019"),
                    LineItem(description="Prorated credit", line_amount=-10, account_code="200", tax_type="NONE")],
        **kw)


def case_create_invoices_computes_the_totals():
    api, tenant = accounting_api()
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(new_contact(api, tenant))])).invoices[0]
    assert created.invoice_id and created.status == "AUTHORISED" and not created.has_errors
    assert created.total == 15.5 and created.amount_due == 15.5 and created.sub_total == 15.5, "Xero computes the totals; the client's 999 is ignored"
    assert created.date == datetime.date(2026, 9, 1) and created.due_date == datetime.date(2026, 9, 30), "the SDK parsed the msdates"
    assert created.updated_date_utc.year >= 2026 and created.currency_code.value == "USD"
    assert [round(l.line_amount, 2) for l in created.line_items] == [25.5, -10.0] and all(l.line_item_id for l in created.line_items)
    assert created.contact.contact_id and created.invoice_number.startswith("INV-")


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
    ("create_invoices computes the totals", case_create_invoices_computes_the_totals),
    ("get_contacts parses with and without page", case_get_contacts_parses_with_and_without_page),
    ("get_contacts with where AccountNumber, as finance-ops calls it", case_get_contacts_as_finance_ops_calls_it),
    ("a duplicate contact name is refused with the words finance-ops looks for", case_duplicate_contact_name_is_refused),
    ("create_contacts sends PUT /Contacts and the SDK parses the contact", case_create_contacts),
    ("the chart of accounts carries the finance-ops accounts", case_chart_of_accounts),
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
