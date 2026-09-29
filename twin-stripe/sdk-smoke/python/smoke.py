"""SDK smoke suite: runs each MVP use case of the Stripe app emulator through
the official stripe-python SDK against a running binary.

    STRIPE_EMULATOR_URL=http://localhost:4111 uv run smoke.py

Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
non-zero if any case fails. run.sh drives both SDK suites.
"""

import json
import os
import sys
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, HTTPServer

import stripe

BASE = os.environ.get("STRIPE_EMULATOR_URL", "http://localhost:4111")
KEY = "sk_test_sdk_smoke_python"
SDK = f"stripe-python {stripe.VERSION}"


def client(key=KEY, **kwargs):
    return stripe.StripeClient(key, base_addresses={"api": BASE}, max_network_retries=0, **kwargs)


sc = client()
v1 = sc.v1


def expect_error(fn, cls):
    try:
        fn()
    except cls as err:
        return err
    raise AssertionError(f"expected {cls.__name__}")


def eq(got, want, what=""):
    if got != want:
        raise AssertionError(f"{what}: expected {want!r}, got {got!r}")


def case_card_payment():
    customer = v1.customers.create({"email": "jenny.rosen@example.com", "name": "Jenny Rosen"})
    pi = v1.payment_intents.create(
        {
            "amount": 2000,
            "currency": "usd",
            "customer": customer.id,
            "payment_method": "pm_card_visa",
            "confirm": True,
            "automatic_payment_methods": {"enabled": True, "allow_redirects": "never"},
        }
    )
    eq(pi.status, "succeeded", "status")
    eq(pi.amount_received, 2000, "amount_received")
    read = v1.payment_intents.retrieve(pi.id, {"expand": ["customer"]})
    eq(read.status, "succeeded", "read status")
    eq(read.customer.id, customer.id, "expanded customer")
    eq(read.customer.object, "customer", "expanded object")


def case_card_decline():
    pi = v1.payment_intents.create({"amount": 1099, "currency": "usd", "payment_method_types": ["card"]})
    eq(pi.status, "requires_payment_method", "status")
    err = expect_error(
        lambda: v1.payment_intents.confirm(pi.id, {"payment_method": "pm_card_visa_chargeDeclined"}),
        stripe.CardError,
    )
    eq(err.http_status, 402, "http status")
    eq(err.code, "card_declined", "code")
    eq(err.error.decline_code, "generic_decline", "decline_code")
    read = v1.payment_intents.retrieve(pi.id)
    eq(read.status, "requires_payment_method", "status after decline")
    eq(read.last_payment_error.code, "card_declined", "last_payment_error")


def case_subscription_billing():
    product = v1.products.create({"name": "Starter plan"})
    price = v1.prices.create(
        {"product": product.id, "unit_amount": 1500, "currency": "usd", "recurring": {"interval": "month"}}
    )
    eq(price.type, "recurring", "price type")
    customer = v1.customers.create({"email": "billing@example.com"})
    pm = v1.payment_methods.attach("pm_card_visa", {"customer": customer.id})
    eq(pm.customer, customer.id, "attached")
    v1.customers.update(customer.id, {"invoice_settings": {"default_payment_method": pm.id}})
    sub = v1.subscriptions.create({"customer": customer.id, "items": [{"price": price.id}]})
    eq(sub.status, "active", "subscription status")
    eq(isinstance(sub.discounts, list), True, "dahlia discounts[]")
    invoices = v1.invoices.list({"subscription": sub.id, "expand": ["data.customer"]})
    inv = invoices.data[0]
    eq(inv.id, sub.latest_invoice, "first invoice")
    eq(inv.status, "paid", "invoice status")
    eq(inv.amount_paid, 1500, "amount_paid")
    eq(inv.customer.id, customer.id, "expanded customer")


def case_checkout_session():
    session = v1.checkout.sessions.create(
        {
            "mode": "payment",
            "ui_mode": "hosted_page",
            "success_url": "https://example.com/success",
            "line_items": [
                {
                    "price_data": {"currency": "usd", "unit_amount": 2500, "product_data": {"name": "T-shirt"}},
                    "quantity": 2,
                }
            ],
        }
    )
    eq(session.status, "open", "status")
    eq(session.amount_total, 5000, "amount_total")
    with_items = v1.checkout.sessions.retrieve(session.id, {"expand": ["line_items"]})
    eq(with_items.line_items.object, "list", "line_items object")
    eq(with_items.line_items.data[0].quantity, 2, "quantity")
    items = v1.checkout.sessions.line_items.list(session.id)
    eq(items.data[0].amount_total, 5000, "line amount_total")
    eq(items.has_more, False, "has_more")


def case_webhook_endpoint_events():
    received = []

    class Receiver(BaseHTTPRequestHandler):
        def do_POST(self):
            body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
            received.append((body, self.headers.get("Stripe-Signature")))
            self.send_response(200)
            self.end_headers()

        def log_message(self, *args):
            pass

    server = HTTPServer(("127.0.0.1", 0), Receiver)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        url = f"http://127.0.0.1:{server.server_port}/hook"
        endpoint = v1.webhook_endpoints.create(
            {"url": url, "enabled_events": ["customer.created"], "api_version": "2026-08-26.dahlia"}
        )
        eq(endpoint.api_version, "2026-08-26.dahlia", "endpoint api_version")
        eq(list(endpoint.enabled_events), ["customer.created"], "enabled_events")
        customer = v1.customers.create({"email": "webhook@example.com"})
        events = v1.events.list({"type": "customer.created", "limit": 1})
        eq(events.data[0].data.object.id, customer.id, "event object")
        eq(events.data[0].api_version.endswith(".dahlia"), True, "event api_version")

        for _ in range(50):
            if received:
                break
            time.sleep(0.1)
        eq(len(received), 1, "deliveries")
        evt = sc.construct_event(received[0][0], received[0][1], endpoint.secret)
        eq(evt.type, "customer.created", "delivered type")
        eq(evt.data.object.id, customer.id, "delivered object")
        eq(evt.api_version, "2026-08-26.dahlia", "delivered api_version")

        deleted = v1.webhook_endpoints.delete(endpoint.id)
        eq(deleted.deleted, True, "deleted")
    finally:
        server.shutdown()


def case_refund():
    pi = v1.payment_intents.create(
        {
            "amount": 3000,
            "currency": "usd",
            "payment_method": "pm_card_visa",
            "confirm": True,
            "payment_method_types": ["card"],
        }
    )
    eq(pi.status, "succeeded", "status")
    refund = v1.refunds.create({"payment_intent": pi.id, "amount": 1000})
    eq(refund.status, "succeeded", "refund status")
    eq(refund.amount, 1000, "refund amount")
    charge = v1.charges.retrieve(pi.latest_charge)
    eq(charge.amount_refunded, 1000, "amount_refunded")
    eq(charge.refunded, False, "refunded")


def case_expand_and_paginate():
    customer = v1.customers.create({"email": "expand@example.com"})

    def pay(amount):
        return v1.payment_intents.create(
            {
                "amount": amount,
                "currency": "usd",
                "customer": customer.id,
                "payment_method": "pm_card_visa",
                "confirm": True,
                "payment_method_types": ["card"],
            }
        )

    first = pay(1000)
    second = pay(2000)
    page = v1.charges.list({"customer": customer.id, "limit": 1, "expand": ["data.payment_intent.customer"]})
    eq(page.has_more, True, "has_more")
    eq(page.data[0].id, second.latest_charge, "newest first")
    eq(page.data[0].payment_intent.object, "payment_intent", "expanded intent")
    eq(page.data[0].payment_intent.customer.id, customer.id, "deep expansion")
    nxt = v1.charges.list({"customer": customer.id, "limit": 1, "starting_after": page.data[0].id})
    eq(nxt.has_more, False, "last page")
    eq(nxt.data[0].id, first.latest_charge, "older charge")
    err = expect_error(
        lambda: v1.charges.list({"expand": ["data.payment_intent.customer.default_source.customer"]}),
        stripe.InvalidRequestError,
    )
    eq(err.http_status, 400, "depth refusal")


def case_idempotent_retry():
    key = f"smoke-{uuid.uuid4()}"
    first = v1.customers.create({"email": "retry@example.com"}, {"idempotency_key": key})
    again = v1.customers.create({"email": "retry@example.com"}, {"idempotency_key": key})
    eq(again.id, first.id, "replayed")
    expect_error(
        lambda: v1.customers.create({"email": "someone-else@example.com"}, {"idempotency_key": key}),
        stripe.IdempotencyError,
    )
    listed = v1.customers.list({"email": "retry@example.com"})
    eq(listed.data[0].id, first.id, "one customer")


def case_connect_transfer():
    account = v1.accounts.create({"type": "express", "country": "US", "email": "seller@example.com"})
    eq(account.object, "account", "account")
    # Transfers draw on the available balance; this test method funds it directly.
    pi = v1.payment_intents.create(
        {
            "amount": 5000,
            "currency": "usd",
            "payment_method": "pm_card_bypassPending",
            "confirm": True,
            "payment_method_types": ["card"],
        }
    )
    eq(pi.status, "succeeded", "funding payment")
    transfer = v1.transfers.create({"amount": 1000, "currency": "usd", "destination": account.id})
    eq(transfer.destination, account.id, "destination")
    read = v1.transfers.retrieve(transfer.id)
    eq(read.amount, 1000, "transfer amount")
    balance = v1.balance.retrieve()
    eq(balance.object, "balance", "balance")


def case_error_bad_key():
    err = expect_error(lambda: client("sk_live_not_a_test_key").v1.customers.list(), stripe.AuthenticationError)
    eq(err.http_status, 401, "status")


def case_error_missing_resource():
    err = expect_error(lambda: v1.customers.retrieve("cus_does_not_exist"), stripe.InvalidRequestError)
    eq(err.http_status, 404, "status")
    eq(err.code, "resource_missing", "code")
    eq(err.param, "id", "param")
    eq(bool(err.error.doc_url), True, "doc_url")


def case_error_declined_card():
    err = expect_error(
        lambda: v1.payment_intents.create(
            {
                "amount": 500,
                "currency": "usd",
                "confirm": True,
                "payment_method": "pm_card_visa_chargeDeclinedInsufficientFunds",
            }
        ),
        stripe.CardError,
    )
    eq(err.error.decline_code, "insufficient_funds", "decline_code")


def case_error_idempotency_mismatch():
    key = f"mismatch-{uuid.uuid4()}"
    v1.products.create({"name": "A"}, {"idempotency_key": key})
    expect_error(lambda: v1.products.create({"name": "B"}, {"idempotency_key": key}), stripe.IdempotencyError)


def case_error_unsupported_version():
    old = client(stripe_version="2025-09-30.clover")
    err = expect_error(lambda: old.v1.customers.list(), stripe.InvalidRequestError)
    eq(err.http_status, 400, "status")


CASES = {
    "stripe-card-payment": case_card_payment,
    "stripe-card-decline": case_card_decline,
    "stripe-subscription-billing": case_subscription_billing,
    "stripe-checkout-session": case_checkout_session,
    "stripe-webhook-endpoint-events": case_webhook_endpoint_events,
    "stripe-refund": case_refund,
    "stripe-expand-and-paginate": case_expand_and_paginate,
    "stripe-idempotent-retry": case_idempotent_retry,
    "stripe-connect-transfer": case_connect_transfer,
    "error-bad-key": case_error_bad_key,
    "error-missing-resource": case_error_missing_resource,
    "error-declined-card": case_error_declined_card,
    "error-idempotency-mismatch": case_error_idempotency_mismatch,
    "error-unsupported-version": case_error_unsupported_version,
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
