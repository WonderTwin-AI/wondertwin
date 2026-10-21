"""SDK smoke suite: runs the Xero app emulator's token and connection flows
through the official xero-python SDK against a running binary.

    XERO_EMULATOR_URL=http://localhost:4195 uv run smoke.py

The SDK hard-codes https://identity.xero.com in TokenApi, so the suite points
its URL attributes at the emulator. The SDK has no authorization-code helper
(the sample app does that with a third-party OAuth library), so the first
refresh token comes from a plain form POST. Prints one JSON line per case
({"sdk","case","pass","detail"}) and exits non-zero if any case fails.
"""

import base64
import datetime
import hashlib
import hmac
import http.client
import http.server
import json
import os
import sys
import threading
import time
import urllib.parse
import uuid

from xero_python.api_client import ApiClient, Configuration
from xero_python.api_client.oauth2 import OAuth2Token, TokenApi
from xero_python.accounting import (Account, AccountingApi, Address, Allocation, Contact, Contacts, CreditNote, CreditNotes,
                                    CurrencyCode, Invoice, Invoices, LineItem, Payment, PaymentDelete)
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


def stored_invoice(invoice_id):
    """The invoice as the emulator holds it, from GET /admin/state (there is no GET /Invoices yet)."""
    conn = http.client.HTTPConnection(urllib.parse.urlsplit(BASE).netloc)
    conn.request("GET", "/admin/state")
    state = conn.getresponse().read().decode()

    def walk(node):
        if isinstance(node, dict) and node.get("InvoiceID") == invoice_id and "AmountCredited" in node:
            return node
        for v in (node.values() if isinstance(node, dict) else node if isinstance(node, list) else ()):
            if (hit := walk(v)) is not None:
                return hit

    return walk(json.loads(state))


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


def case_create_invoices_refusals():
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    bad = finance_ops_invoice(contact)
    bad.line_items[0].account_code = "999"
    try:
        api.create_invoices(tenant, Invoices(invoices=[bad]))
        raise AssertionError("an unknown account code was accepted")
    except AssertionError:
        raise
    except Exception as e:  # the SDK raises on a 400 with the body in the message
        assert "not a valid code" in str(e), str(e)
    number = f"SMOKE-{uuid.uuid4().hex[:6]}"
    api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact, invoice_number=number)]))
    try:
        api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact, invoice_number=number)]))
        raise AssertionError("a duplicate invoice number was accepted")
    except AssertionError:
        raise
    except Exception as e:  # finance-ops looks for these words to add a suffix and retry
        assert "Invoice # must be unique" in str(e), str(e)


def case_create_invoices_summarize_errors_false():
    """With summarize_errors=False one invalid invoice does not fail the batch: the SDK gets 200 and each
    invoice back, the bad one with has_errors and its validation errors (what finance-ops checks for)."""
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    bad = finance_ops_invoice(contact)
    bad.line_items[0].account_code = "999"
    result = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact), bad]), summarize_errors=False).invoices
    assert len(result) == 2 and result[0].invoice_id and not result[0].has_errors
    assert result[1].has_errors and result[1].validation_errors and "not a valid code" in result[1].validation_errors[0].message
    assert not result[1].invoice_id, "a refused invoice is not created"


def case_update_invoice_as_finance_ops_does():
    """finance-ops reads an invoice, changes a field and posts the whole model back with update_invoice."""
    api, tenant = accounting_api()
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(new_contact(api, tenant))])).invoices[0]
    created.reference = "updated by smoke"
    updated = api.update_invoice(tenant, invoice_id=created.invoice_id, invoices=Invoices(invoices=[created])).invoices[0]
    assert updated.reference == "updated by smoke" and updated.invoice_id == created.invoice_id and updated.total == created.total
    assert updated.updated_date_utc >= created.updated_date_utc
    by_number = api.update_invoice(tenant, invoice_id=created.invoice_number, invoices=Invoices(invoices=[Invoice(reference="by number")]))
    assert by_number.invoices[0].reference == "by number", "the path may name the invoice by its number"


def case_get_invoices_as_finance_ops_calls_it():
    """finance-ops looks for an existing invoice with get_invoices(contact_i_ds=[...], page=1) and matches by number."""
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    number = f"LIST-{uuid.uuid4().hex[:6]}"
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact, invoice_number=number)])).invoices[0]
    found = api.get_invoices(tenant, contact_i_ds=[contact], page=1)
    assert [i.invoice_number for i in found.invoices] == [number] and found.invoices[0].invoice_id == created.invoice_id
    assert found.pagination.page == 1 and found.pagination.item_count == 1, "the SDK parsed the pagination object"
    assert found.invoices[0].line_items and found.invoices[0].total == 15.5 and found.invoices[0].date == datetime.date(2026, 9, 1)
    assert api.get_invoices(tenant, contact_i_ds=[new_contact(api, tenant)], page=1).invoices == [], "an unrelated contact has none"


def case_get_invoices_by_number_where_and_read():
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    number = f"READ-{uuid.uuid4().hex[:6]}"
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact, invoice_number=number)])).invoices[0]
    by_number = api.get_invoices(tenant, invoice_numbers=[number], page=1).invoices
    assert [i.invoice_id for i in by_number] == [created.invoice_id], "the migration oracle's lookup by InvoiceNumber"
    by_where = api.get_invoices(tenant, where=f'InvoiceNumber="{number}"', page=1).invoices
    assert [i.invoice_id for i in by_where] == [created.invoice_id]
    single = api.get_invoice(tenant, created.invoice_id).invoices[0]
    assert single.invoice_number == number and single.line_items and single.contact.contact_id == contact
    unpaged = api.get_invoices(tenant, invoice_numbers=[number])
    assert unpaged.pagination is None and not unpaged.invoices[0].line_items, "no page: summary form, and no [] for the SDK to choke on"


def case_create_payment_pays_the_invoice():
    """create_payment as localstack/finance-ops calls it: the amount is the invoice total, the account
    is by code, the whole invoice is nested, and the payment type is ACCRECPAYMENT."""
    api, tenant = accounting_api()
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(new_contact(api, tenant))])).invoices[0]
    payment = Payment(amount=created.total, account=Account(code="090"), date=created.date, payment_type="ACCRECPAYMENT", invoice=created)
    paid = api.create_payment(tenant, payment).payments[0]
    assert paid.payment_id and paid.status == "AUTHORISED" and paid.payment_type == "ACCRECPAYMENT" and paid.has_account
    assert paid.date == created.date and paid.updated_date_utc.year >= 2026, "the SDK parsed the msdates"
    assert paid.invoice.status == "PAID" and paid.invoice.amount_due == 0 and paid.invoice.amount_paid == created.total
    assert paid.invoice.fully_paid_on_date == created.date
    try:
        api.create_payment(tenant, payment)
        raise AssertionError("a second payment against a paid invoice was accepted")
    except AssertionError:
        raise
    except Exception as e:  # the SDK raises on a 400 with the body in the message
        assert "not of valid status" in str(e), str(e)


def case_delete_payment_restores_the_invoice():
    """delete_payment posts {Status: DELETED} to /Payments/{id}: the invoice goes back to AUTHORISED with its amount due."""
    api, tenant = accounting_api()
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(new_contact(api, tenant))])).invoices[0]
    paid = api.create_payment(tenant, Payment(amount=created.total, account=Account(code="090"), date=created.date, invoice=created)).payments[0]
    gone = api.delete_payment(tenant, payment_id=paid.payment_id, payment_delete=PaymentDelete(status="DELETED")).payments[0]
    assert gone.status == "DELETED" and gone.payment_id == paid.payment_id
    assert gone.invoice.status == "AUTHORISED" and gone.invoice.amount_due == created.total and gone.invoice.amount_paid == 0
    assert api.get_invoice(tenant, created.invoice_id).invoices[0].status == "AUTHORISED"
    try:
        api.delete_payment(tenant, payment_id=paid.payment_id, payment_delete=PaymentDelete(status="DELETED"))
        raise AssertionError("a payment was deleted twice")
    except AssertionError:
        raise
    except Exception as e:
        assert "already been deleted" in str(e), str(e)


def case_refund_credit_note_payment_as_finance_ops_does():
    """The Stripe refund sequence in xero_client.py: the invoice is already paid, one create_credit_notes carries the
    full Allocation (the invoice object as read back), then create_payment(credit_note, account) pays the credit
    note's total as ARCREDITPAYMENT, with the credit note object as the create call returned it."""
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    invoice = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact)])).invoices[0]
    api.create_payment(tenant, Payment(amount=invoice.total, account=Account(code="090"), date=invoice.date, payment_type="ACCRECPAYMENT", invoice=invoice))
    refund = finance_ops_credit_note(contact, amount=invoice.total)
    refund.allocations = [Allocation(invoice=invoice, amount=invoice.total, date=datetime.date(2026, 9, 11))]
    credit_note = api.create_credit_notes(tenant, CreditNotes(credit_notes=[refund])).credit_notes[0]
    payment = Payment(amount=credit_note.total, account=Account(code="090"), date=credit_note.date, credit_note=credit_note, payment_type="ARCREDITPAYMENT")
    paid = api.create_payment(tenant, payment).payments[0]
    assert paid.payment_id and paid.payment_type == "ARCREDITPAYMENT" and paid.amount == invoice.total and paid.has_account
    assert paid.credit_note.credit_note_id == credit_note.credit_note_id and paid.credit_note.status == "PAID", paid.credit_note
    assert paid.credit_note.remaining_credit == 0 and len(paid.credit_note.payments) == 1 and paid.invoice is None


def case_update_invoice_line_items():
    """The SDK posts the lines back with their IDs: an edited line keeps its ID and the totals are recomputed."""
    api, tenant = accounting_api()
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(new_contact(api, tenant))])).invoices[0]
    first = created.line_items[0]
    first.quantity, first.line_amount = (first.quantity or 1) + 1, None  # a client that edits a line drops its stale LineAmount
    updated = api.update_invoice(tenant, invoice_id=created.invoice_id, invoices=Invoices(invoices=[created])).invoices[0]
    assert updated.line_items[0].line_item_id == first.line_item_id, "an edited line keeps its LineItemID"
    assert updated.total != created.total and updated.amount_due == updated.total, "the totals are recomputed"


def case_void_and_delete_invoices_as_finance_ops_does():
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    created = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact)])).invoices[0]
    created.status = "VOIDED"  # finance-ops delete_invoice(): set the status and post the whole model back
    voided = api.update_invoice(tenant, invoice_id=created.invoice_id, invoices=Invoices(invoices=[created])).invoices[0]
    assert voided.status == "VOIDED" and voided.amount_due == 0 and voided.total == 15.5
    draft = finance_ops_invoice(contact)
    draft.status = "DRAFT"
    made = api.create_invoices(tenant, Invoices(invoices=[draft])).invoices[0]
    gone = api.update_invoice(tenant, invoice_id=made.invoice_number, invoices=Invoices(invoices=[Invoice(invoice_number=made.invoice_number, status="DELETED")]))
    assert gone.invoices[0].status == "DELETED", "deleting a draft is a status write"
    try:
        api.update_invoice(tenant, invoice_id=created.invoice_id, invoices=Invoices(invoices=[Invoice(reference="too late")]))
        raise AssertionError("a voided invoice was updated")
    except AssertionError:
        raise
    except Exception as e:
        assert "cannot be updated" in str(e), str(e)


def finance_ops_credit_note(contact_id, amount=20.0, **kw):
    """The shape localstack/finance-ops sends for a Stripe refund: its own number, one line with an empty
    LineItemID, and a client-side Total and SubTotal that Xero ignores."""
    return CreditNote(
        type="ACCRECCREDIT", status="AUTHORISED", contact=Contact(contact_id=contact_id), currency_code=CurrencyCode("USD"),
        credit_note_number=f"Stripe refund re_{uuid.uuid4().hex[:8]}", date=datetime.date(2026, 9, 10), reference="in_smoke",
        total=amount, sub_total=amount,
        line_items=[LineItem(line_item_id="", quantity=1, unit_amount=amount, line_amount=amount, account_code="200",
                             tax_type="NONE", description="Stripe refund")], **kw)


def case_create_credit_notes():
    api, tenant = accounting_api()
    created = api.create_credit_notes(tenant, CreditNotes(credit_notes=[finance_ops_credit_note(new_contact(api, tenant), amount=20)])).credit_notes[0]
    assert created.credit_note_id and created.status == "AUTHORISED" and not created.has_errors
    assert created.total == 20 and created.remaining_credit == 20 and not created.applied_amount and not created.allocations
    assert created.date == datetime.date(2026, 9, 10) and created.updated_date_utc.year >= 2026, "the SDK parsed the msdates"
    assert created.credit_note_number.startswith("Stripe refund re_") and created.contact.contact_id
    assert created.line_items[0].line_item_id, "an empty LineItemID is replaced"


def case_create_credit_notes_with_allocation():
    """The finance-ops refund: the Allocation inline, against an invoice that is already PAID."""
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    invoice = api.create_invoices(tenant, Invoices(invoices=[Invoice(
        type="ACCREC", status="AUTHORISED", contact=Contact(contact_id=contact), date=datetime.date(2026, 9, 1),
        due_date=datetime.date(2026, 9, 30), line_items=[LineItem(quantity=1, unit_amount=100, account_code="200")])])).invoices[0]
    when = datetime.date(2026, 9, 11)
    first = finance_ops_credit_note(contact, amount=100)
    first.allocations = [Allocation(invoice=Invoice(invoice_id=invoice.invoice_id), amount=100, date=when)]
    api.create_credit_notes(tenant, CreditNotes(credit_notes=[first]))  # pays the invoice in full
    refund = finance_ops_credit_note(contact, amount=30)
    refund.allocations = [Allocation(invoice=invoice, amount=30, date=when)]  # the whole invoice object, as finance-ops sends it
    created = api.create_credit_notes(tenant, CreditNotes(credit_notes=[refund])).credit_notes[0]
    assert created.status == "PAID" and created.remaining_credit == 0 and created.applied_amount == 30
    assert created.allocations[0].allocation_id and created.allocations[0].date == when
    after = stored_invoice(invoice.invoice_id)
    assert after["AmountCredited"] == 130 and after["AmountDue"] == 0 and after["Status"] == "PAID", after


def case_get_credit_notes_as_finance_ops_calls_it():
    """finance-ops checks whether a refund was booked: get_credit_notes(page=1), then a number match."""
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    refund = finance_ops_credit_note(contact)
    api.create_credit_notes(tenant, CreditNotes(credit_notes=[refund]))
    existing = api.get_credit_notes(tenant, page=1)
    assert existing.pagination.page == 1 and existing.pagination.item_count >= 1
    match = [cn for cn in existing.credit_notes if refund.credit_note_number in str(cn.credit_note_number)]
    assert match and match[0].contact.contact_id == contact and match[0].line_items and match[0].total == 20
    filtered = api.get_credit_notes(tenant, where=f'Contact.ContactID==Guid("{contact}")').credit_notes
    assert [cn.credit_note_number for cn in filtered] == [refund.credit_note_number]


def case_create_credit_notes_refusals():
    api, tenant = accounting_api()
    contact = new_contact(api, tenant)
    bad = finance_ops_credit_note(contact)
    bad.line_items[0].account_code = "999"
    try:
        api.create_credit_notes(tenant, CreditNotes(credit_notes=[bad]))
        raise AssertionError("an unknown account code was accepted")
    except AssertionError:
        raise
    except Exception as e:  # the SDK raises on a 400 with the body in the message
        assert "not a valid code" in str(e), str(e)
    mixed = api.create_credit_notes(tenant, CreditNotes(credit_notes=[finance_ops_credit_note(contact), bad]), summarize_errors=False).credit_notes
    assert mixed[0].credit_note_id and not mixed[0].has_errors and mixed[1].has_errors and not mixed[1].credit_note_id


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


def case_webhook_events_follow_the_writes():
    """A receiver registered the way the developer portal would, the way a Xero app answers the
    intent-to-receive check, then writes through the SDK and signed events back."""
    key, events, subscription = "smoke-webhook-key", [], None

    class Receiver(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            body = self.rfile.read(int(self.headers["Content-Length"]))
            want = base64.b64encode(hmac.new(key.encode(), body, hashlib.sha256).digest()).decode()
            ok = hmac.compare_digest(want, self.headers.get("x-xero-signature", ""))
            self.send_response(200 if ok else 401)
            self.end_headers()
            if ok:
                events.extend(json.loads(body)["events"])

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Receiver)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        conn = http.client.HTTPConnection(urllib.parse.urlsplit(BASE).netloc)
        conn.request("POST", "/admin/webhooks", json.dumps({"url": f"http://127.0.0.1:{server.server_port}/", "key": key}),
                     {"Content-Type": "application/json"})
        registered = conn.getresponse()
        subscription = json.loads(registered.read())
        assert registered.status == 201 and subscription["status"] == "OK", "the intent-to-receive check passed"

        api, tenant = accounting_api()
        contact_id = new_contact(api, tenant)
        invoice_id = api.create_invoices(tenant, Invoices(invoices=[finance_ops_invoice(contact_id)])).invoices[0].invoice_id
        note_id = api.create_credit_notes(tenant, CreditNotes(credit_notes=[finance_ops_credit_note(contact_id)])).credit_notes[0].credit_note_id
        for _ in range(100):
            if {e["resourceId"] for e in events} >= {contact_id, invoice_id, note_id}:
                break
            time.sleep(0.05)
        seen = {(e["eventCategory"], e["eventType"]): e for e in events if e["resourceId"] in (contact_id, invoice_id, note_id)}
        assert set(seen) == {("CONTACT", "CREATE"), ("INVOICE", "CREATE"), ("CREDITNOTE", "CREATE")}, seen
        assert seen[("CONTACT", "CREATE")]["resourceId"] == contact_id and seen[("INVOICE", "CREATE")]["resourceId"] == invoice_id
        assert seen[("CREDITNOTE", "CREATE")]["data"] == {"Type": "ACCRECCREDIT", "Status": "AUTHORISED"}
        assert all(e["tenantId"] == tenant and e["tenantType"] == "ORGANISATION" for e in seen.values())
        assert seen[("INVOICE", "CREATE")]["resourceUrl"].endswith(f"/api.xro/2.0/Invoices/{invoice_id}")
        called_back = api.get_invoice(tenant, invoice_id).invoices[0]
        assert called_back.invoice_id == invoice_id, "resourceUrl points at a resource the SDK can read back"
        api.update_or_create_contacts(tenant, Contacts(contacts=[Contact(contact_id=contact_id, email_address="events@example.test")]))
        for _ in range(100):
            if ("CONTACT", "UPDATE") in {(e["eventCategory"], e["eventType"]) for e in events if e["resourceId"] == contact_id}:
                break
            time.sleep(0.05)
        assert ("CONTACT", "UPDATE") in {(e["eventCategory"], e["eventType"]) for e in events if e["resourceId"] == contact_id}
    finally:
        if subscription:  # later cases write too, and must not queue deliveries for a receiver that is gone
            conn.request("DELETE", f"/admin/webhooks/{subscription['id']}")
            conn.getresponse().read()
        server.shutdown()


def case_legacy_control_paths_alias_the_admin_plane():
    def raw(method, path):
        conn = http.client.HTTPConnection(urllib.parse.urlsplit(BASE).netloc)
        conn.request(method, path)
        res = conn.getresponse()
        return res.status, json.loads(res.read() or b"null")

    assert raw("GET", "/_xero/health") == (200, {"status": "ok"})
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    api.create_contacts(tenant, Contacts(contacts=[Contact(name=f"Alias {uuid.uuid4().hex[:8]}")]), summarize_errors=False)
    assert api.get_contacts(tenant, page=1).contacts, "the write is visible before the reset"
    assert raw("POST", "/_xero/reset") == (200, {"status": "reset"})
    client, _, _ = signed_in()  # a reset drops the tokens too
    api, tenant = accounting_for(client)
    assert not api.get_contacts(tenant, page=1).contacts, "the reset cleared the store"


def case_update_or_create_contacts():
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    created = api.update_or_create_contacts(tenant, Contacts(contacts=[Contact(name="Smoke Post", account_number="post_1", email_address="p@example.test")])).contacts
    assert len(created) == 1 and created[0].contact_id and created[0].contact_status == "ACTIVE"
    updated = api.update_or_create_contacts(tenant, Contacts(contacts=[Contact(contact_id=created[0].contact_id, name="Smoke Post Ltd")])).contacts
    assert len(updated) == 1 and updated[0].contact_id == created[0].contact_id and updated[0].name == "Smoke Post Ltd"
    assert updated[0].account_number == "post_1" and updated[0].email_address == "p@example.test", "an update leaves the other fields"
    assert len([c for c in api.get_contacts(tenant, page=1).contacts if c.contact_id == created[0].contact_id]) == 1


def case_update_or_create_contacts_never_upserts_by_name():
    """crud-009: a Name that belongs to another contact and no ContactID is refused as PUT refuses it."""
    client, _, _ = signed_in()
    api, tenant = accounting_for(client)
    api.create_contacts(tenant, Contacts(contacts=[Contact(name="Smoke Post Dup")]))
    try:
        api.update_or_create_contacts(tenant, Contacts(contacts=[Contact(name="smoke post dup")]))
    except Exception as e:  # the SDK raises on a 400
        assert "contact name" in str(e).lower(), str(e)[:300]
        return
    raise AssertionError("POST /Contacts upserted by Name")


CASES = [
    ("webhook events follow the writes: signed CONTACT, INVOICE and CREDITNOTE events", case_webhook_events_follow_the_writes),
    ("a refund credit note is allocated, then paid in full with ARCREDITPAYMENT, as finance-ops does", case_refund_credit_note_payment_as_finance_ops_does),
    ("get_credit_notes page=1 as finance-ops calls it, and where by contact", case_get_credit_notes_as_finance_ops_calls_it),
    ("update_invoice voids and deletes as finance-ops does", case_void_and_delete_invoices_as_finance_ops_does),
    ("update_invoice recomputes the totals when a line changes", case_update_invoice_line_items),
    ("update_invoice takes a whole invoice back, by ID or number", case_update_invoice_as_finance_ops_does),
    ("get_invoices as finance-ops calls it, by contact with page=1", case_get_invoices_as_finance_ops_calls_it),
    ("get_invoices by number and where, get_invoice, and the unpaged summary form", case_get_invoices_by_number_where_and_read),


    ("create_payment pays the invoice, as finance-ops calls it", case_create_payment_pays_the_invoice),
    ("create_credit_notes with an inline Allocation against an invoice", case_create_credit_notes_with_allocation),
    ("create_credit_notes refuses an unknown account and answers in place with summarize_errors=False", case_create_credit_notes_refusals),
    ("create_credit_notes parses a finance-ops refund credit note", case_create_credit_notes),
    ("delete_payment restores the invoice", case_delete_payment_restores_the_invoice),
    ("create_invoices computes the totals", case_create_invoices_computes_the_totals),
    ("create_invoices refuses an unknown account and a duplicate number", case_create_invoices_refusals),
    ("create_invoices with summarize_errors=False answers each invoice in place", case_create_invoices_summarize_errors_false),
    ("get_contacts parses with and without page", case_get_contacts_parses_with_and_without_page),
    ("get_contacts with where AccountNumber, as finance-ops calls it", case_get_contacts_as_finance_ops_calls_it),
    ("a duplicate contact name is refused with the words finance-ops looks for", case_duplicate_contact_name_is_refused),
    ("create_contacts sends PUT /Contacts and the SDK parses the contact", case_create_contacts),
    ("update_or_create_contacts creates, then updates by ContactID and keeps the other fields", case_update_or_create_contacts),
    ("update_or_create_contacts never upserts by Name", case_update_or_create_contacts_never_upserts_by_name),
    ("the chart of accounts carries the finance-ops accounts", case_chart_of_accounts),
    ("made-up tokens are accepted by default", case_made_up_tokens_are_accepted_by_default),
    ("strict auth refuses made-up tokens", case_strict_auth_refuses_made_up_tokens),
    ("connect: a code exchange, then get_connections returns the tenant", case_connect_tenant),
    ("refresh rotates the token", case_refresh_rotates_the_token),
    ("a replayed refresh token is refused", case_replayed_refresh_token_is_refused),
    ("an expired token refreshes on demand", case_expired_token_refreshes_on_demand),
    ("client credentials", case_client_credentials),
    ("the legacy /_xero health and reset paths alias the admin plane (last: it resets the store)", case_legacy_control_paths_alias_the_admin_plane),
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
