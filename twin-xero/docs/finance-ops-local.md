# Running localstack/finance-ops against twin-xero

`twin-xero` replaces the `localstack/xero-local` image for the finance demo app. This page is for checking that locally before the `localstack/finance-ops` change is opened. Written against `localstack/finance-ops` at `8ff5f6bfdb5c517da8d8378889d914e6d4ab1004`. Nothing here modifies `finance-ops`; the changes it needs are listed at the end.

## 1. Build and start the twin

From a checkout of `WonderTwin-AI/wondertwin`:

```sh
go build -o twin-xero ./twin-xero/cmd/twin-xero
./twin-xero --port=5001
```

`5001` is the port `xero-local` used, so `finance-ops` needs no URL change. The default is `4115`. Leave `--strict-auth` off: `finance-ops` seeds made-up tokens and client credentials, and strict mode refuses them as Xero does (divergences `auth-001`, `auth-003`, `auth-006`, `auth-007`).

Nothing needs seeding. The default tenant answers to `00000000-0000-0000-0000-000000000001` (an unknown tenant id is accepted by default) and carries the chart accounts `090`, `200`, `201` and `404`. A made-up bearer token and a made-up refresh token are accepted; `POST /connect/token` answers a Xero-shaped token.

## 2. Point finance-ops at it

| Variable | Value | Notes |
|---|---|---|
| `XERO_BASE_URL` | `http://localhost:5001` | the SDK appends `/api.xro/2.0` |
| `XERO_TOKEN_URL` | `http://localhost:5001/connect/token` | |
| `XERO_TENANT_ID` | `00000000-0000-0000-0000-000000000001` | `tests/conftest.py` sets it |

Both URLs are `tests/conftest.py` defaults, so port `5001` needs no export.

## 3. What finance-ops calls

- `Makefile` `services-wait` polls `GET /_xero/health`. `twin-xero` serves it as an alias of `GET /admin/health` (compat-001).
- `tests/conftest.py` `reset_xero_state` posts `/_xero/reset` before and after every test. Served as an alias of `POST /admin/reset` (compat-001). It wipes every record, so nothing created in one test is visible in the next.
- `tests/conftest.py` `xero_local_server` skips starting `xero_local` when `/_xero/health` already answers 200, so the twin is used as is.
- `TestXeroLocal` (`tests/test_migration.py`, about lines 372 to 490) sends raw HTTP: `POST /Contacts`, `/Invoices`, `/Payments`, `/CreditNotes`. All four are served as Xero's update-or-create (crud-009 to crud-011).
- `TestXeroClientE2E` and the Lambda use xero-python: `get_contacts(where='AccountNumber="x"', page=1)`, `create_contacts`, `create_invoices`, `get_invoices(contact_i_ds=[...], page=1)`, `create_payment`, `create_credit_notes` with an inline Allocation, `create_payment` with `ARCREDITPAYMENT`, `get_credit_notes(page=1)`, `delete_payment`.

## 4. Run it

With the twin up on `5001`, in a `finance-ops` checkout with its Python dependencies:

```sh
pytest tests/test_migration.py -k "TestXeroLocal or TestXeroClientE2E or TestLineItemMath or TestBillingIntervalDetection"
```

Result when this was written: 29 passed, 1 failed (`test_duplicate_contact_name_upserts`, see below). That run did not start LocalStack or the Stripe twin, so the Stripe and DynamoDB tests were not run. `make test` also needs those; how Xero is started next to them is not settled here (see section 6).

Without `finance-ops`, `twin-xero/sdk-smoke/run.sh` runs the same call sequence through xero-python 15.2.0 (`case_finance_ops_flow_end_to_end`), and `scenarios/xero-finance-ops.json` runs it over raw HTTP with `wt test`.

## 5. What you change in finance-ops

1. **`Makefile` `services-start`.** Replace the `docker run ... localstack/xero-local:latest` line with a command that starts `twin-xero --port=$(XERO_PORT)`, and the matching `docker rm -f xero-local` in `services-stop`. `services-wait` and `conftest.py` work unchanged.
2. **`test_duplicate_contact_name_upserts`.** It posts the same contact `Name` twice and expects 200 both times, and its comment says Xero upserts. `twin-xero` answers the second post with 400, as `PUT` does (GAP-007, crud-009): the research does not settle that Xero upserts by `Name`, so it is not built. Adapt the test, or drop it.
3. **`XERO_STRIPE_CONTACT_ID`.** The fee bill (`_create_fee_bill_invoice` in `xero_client.py`) bills the contact whose id is this setting. `conftest.py` sets `00000000-0000-0000-0000-000000000002`. Xero refuses an invoice for a contact that does not exist, and `twin-xero` does too: it answers 400 `Contact ... could not be found`. `xero-local` never checked. No test in `tests/` runs the fee path, but running the Lambda locally does. Create the contact first (`PUT /Contacts`) and set `XERO_STRIPE_CONTACT_ID` to the id it returns; a reset deletes it, so create it after each reset.

## 6. Not settled here

- **How the twin starts next to LocalStack and the Stripe twin.** The Makefile starts the Stripe twin with `TWINS_ENABLED=stripe` (the Application Twins extension) and `xero-local` as a plain container. This page assumes a plain `twin-xero --port=N` process.
- **Real Xero behaviour for several guessed rules.** Every deliberate difference is in `twin-xero/divergences.json`, each with a quirk id. The ones the demo touches: allocation to a paid invoice and payment of a credit note up to its total (alloc-001, alloc-002), update-or-create matching (crud-009 to crud-011), and `order` and `If-Modified-Since` being ignored (query-001, page-003).

## 7. If something fails

1. `curl -s localhost:5001/admin/health` answers `{"status":"ok"}`. If `/_xero/health` does not, the binary predates `WonderTwin-AI/wondertwin#421`.
2. A 401 means `--strict-auth` is on. Restart without it, or `PUT /admin/config` with `{"strict_auth":false}`.
3. A 400 with a `ValidationException` names the field in `ValidationErrors`; compare it with `divergences.json` first.
4. `curl -s localhost:5001/admin/state` shows what the twin holds. `GET /admin/webhooks/deliveries` shows webhook deliveries.
5. A test that passed against `xero-local` and fails here: check whether it relied on a `xero-local` behaviour in `localstack-extensions-restricted` `xero/docs/implementation.md` (token expiry, case-insensitive duplicate names, `AmountDue` copied from `Total`, no pagination, no `where`).
