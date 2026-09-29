// SDK smoke suite: runs each MVP use case of the Stripe app emulator through
// the official stripe-node SDK against a running binary.
//
//   STRIPE_EMULATOR_URL=http://localhost:4111 node smoke.mjs
//
// Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
// non-zero if any case fails. run.sh drives both SDK suites.

import http from 'node:http';
import assert from 'node:assert/strict';
import Stripe from 'stripe';

const base = new URL(process.env.STRIPE_EMULATOR_URL || 'http://localhost:4111');
const KEY = 'sk_test_sdk_smoke_node';
const sdk = `stripe-node ${Stripe.PACKAGE_VERSION}`;

function client(opts = {}) {
  return new Stripe(opts.key || KEY, {
    host: base.hostname,
    port: Number(base.port || 80),
    protocol: base.protocol.replace(':', ''),
    maxNetworkRetries: 0,
    ...opts.config,
  });
}

const stripe = client();

async function expectError(fn) {
  try {
    await fn();
  } catch (err) {
    return err;
  }
  assert.fail('expected the call to fail');
}

const cases = {
  async 'stripe-card-payment'() {
    const customer = await stripe.customers.create({email: 'jenny.rosen@example.com', name: 'Jenny Rosen'});
    const pi = await stripe.paymentIntents.create({
      amount: 2000,
      currency: 'usd',
      customer: customer.id,
      payment_method: 'pm_card_visa',
      confirm: true,
      automatic_payment_methods: {enabled: true, allow_redirects: 'never'},
    });
    assert.equal(pi.status, 'succeeded');
    assert.equal(pi.amount_received, 2000);
    const read = await stripe.paymentIntents.retrieve(pi.id, {expand: ['customer']});
    assert.equal(read.status, 'succeeded');
    assert.equal(read.customer.id, customer.id);
    assert.equal(read.customer.object, 'customer');
  },

  async 'stripe-card-decline'() {
    const pi = await stripe.paymentIntents.create({amount: 1099, currency: 'usd', payment_method_types: ['card']});
    assert.equal(pi.status, 'requires_payment_method');
    const err = await expectError(() => stripe.paymentIntents.confirm(pi.id, {payment_method: 'pm_card_visa_chargeDeclined'}));
    assert.ok(err instanceof Stripe.errors.StripeCardError, `expected StripeCardError, got ${err.type}`);
    assert.equal(err.statusCode, 402);
    assert.equal(err.code, 'card_declined');
    assert.equal(err.decline_code, 'generic_decline');
    const read = await stripe.paymentIntents.retrieve(pi.id);
    assert.equal(read.status, 'requires_payment_method');
    assert.equal(read.last_payment_error.code, 'card_declined');
  },

  async 'stripe-subscription-billing'() {
    const product = await stripe.products.create({name: 'Starter plan'});
    const price = await stripe.prices.create({product: product.id, unit_amount: 1500, currency: 'usd', recurring: {interval: 'month'}});
    assert.equal(price.type, 'recurring');
    const customer = await stripe.customers.create({email: 'billing@example.com'});
    const pm = await stripe.paymentMethods.attach('pm_card_visa', {customer: customer.id});
    assert.equal(pm.customer, customer.id);
    await stripe.customers.update(customer.id, {invoice_settings: {default_payment_method: pm.id}});
    const sub = await stripe.subscriptions.create({customer: customer.id, items: [{price: price.id}]});
    assert.equal(sub.status, 'active');
    assert.ok(Array.isArray(sub.discounts), 'dahlia subscriptions carry discounts[]');
    const invoices = await stripe.invoices.list({subscription: sub.id, expand: ['data.customer']});
    const inv = invoices.data[0];
    assert.equal(inv.id, sub.latest_invoice);
    assert.equal(inv.status, 'paid');
    assert.equal(inv.amount_paid, 1500);
    assert.equal(inv.customer.id, customer.id);
  },

  async 'stripe-checkout-session'() {
    const session = await stripe.checkout.sessions.create({
      mode: 'payment',
      ui_mode: 'hosted_page',
      success_url: 'https://example.com/success',
      line_items: [{price_data: {currency: 'usd', unit_amount: 2500, product_data: {name: 'T-shirt'}}, quantity: 2}],
    });
    assert.equal(session.status, 'open');
    assert.equal(session.amount_total, 5000);
    const withItems = await stripe.checkout.sessions.retrieve(session.id, {expand: ['line_items']});
    assert.equal(withItems.line_items.object, 'list');
    assert.equal(withItems.line_items.data[0].quantity, 2);
    const items = await stripe.checkout.sessions.listLineItems(session.id);
    assert.equal(items.data[0].amount_total, 5000);
    assert.equal(items.has_more, false);
  },

  async 'stripe-webhook-endpoint-events'() {
    const received = [];
    const server = http.createServer((req, res) => {
      let body = '';
      req.on('data', (c) => (body += c));
      req.on('end', () => {
        received.push({body, sig: req.headers['stripe-signature']});
        res.end('ok');
      });
    });
    await new Promise((r) => server.listen(0, '127.0.0.1', r));
    try {
      const url = `http://127.0.0.1:${server.address().port}/hook`;
      const endpoint = await stripe.webhookEndpoints.create({url, enabled_events: ['customer.created'], api_version: '2026-08-26.dahlia'});
      assert.equal(endpoint.api_version, '2026-08-26.dahlia');
      assert.deepEqual(endpoint.enabled_events, ['customer.created']);
      assert.ok(endpoint.secret);
      const customer = await stripe.customers.create({email: 'webhook@example.com'});
      const events = await stripe.events.list({type: 'customer.created', limit: 1});
      assert.equal(events.data[0].data.object.id, customer.id);
      assert.match(events.data[0].api_version, /\.dahlia$/);

      for (let i = 0; i < 50 && received.length === 0; i++) await new Promise((r) => setTimeout(r, 100));
      assert.equal(received.length, 1, 'expected exactly one delivery to the endpoint');
      const evt = stripe.webhooks.constructEvent(received[0].body, received[0].sig, endpoint.secret);
      assert.equal(evt.type, 'customer.created');
      assert.equal(evt.data.object.id, customer.id);
      assert.equal(evt.api_version, '2026-08-26.dahlia');

      const del = await stripe.webhookEndpoints.del(endpoint.id);
      assert.equal(del.deleted, true);
    } finally {
      server.close();
    }
  },

  async 'stripe-refund'() {
    const pi = await stripe.paymentIntents.create({amount: 3000, currency: 'usd', payment_method: 'pm_card_visa', confirm: true, payment_method_types: ['card']});
    assert.equal(pi.status, 'succeeded');
    const refund = await stripe.refunds.create({payment_intent: pi.id, amount: 1000});
    assert.equal(refund.status, 'succeeded');
    assert.equal(refund.amount, 1000);
    const charge = await stripe.charges.retrieve(pi.latest_charge);
    assert.equal(charge.amount_refunded, 1000);
    assert.equal(charge.refunded, false);
  },

  async 'stripe-expand-and-paginate'() {
    const customer = await stripe.customers.create({email: 'expand@example.com'});
    const pay = (amount) =>
      stripe.paymentIntents.create({amount, currency: 'usd', customer: customer.id, payment_method: 'pm_card_visa', confirm: true, payment_method_types: ['card']});
    const first = await pay(1000);
    const second = await pay(2000);
    const page = await stripe.charges.list({customer: customer.id, limit: 1, expand: ['data.payment_intent.customer']});
    assert.equal(page.has_more, true);
    assert.equal(page.data[0].id, second.latest_charge);
    assert.equal(page.data[0].payment_intent.object, 'payment_intent');
    assert.equal(page.data[0].payment_intent.customer.id, customer.id);
    const next = await stripe.charges.list({customer: customer.id, limit: 1, starting_after: page.data[0].id});
    assert.equal(next.has_more, false);
    assert.equal(next.data[0].id, first.latest_charge);
    const err = await expectError(() => stripe.charges.list({expand: ['data.payment_intent.customer.default_source.customer']}));
    assert.equal(err.statusCode, 400);
    assert.equal(err.type, 'StripeInvalidRequestError');
  },

  async 'stripe-idempotent-retry'() {
    const key = `smoke-${Date.now()}-${Math.random()}`;
    const first = await stripe.customers.create({email: 'retry@example.com'}, {idempotencyKey: key});
    const again = await stripe.customers.create({email: 'retry@example.com'}, {idempotencyKey: key});
    assert.equal(again.id, first.id);
    const err = await expectError(() => stripe.customers.create({email: 'someone-else@example.com'}, {idempotencyKey: key}));
    assert.ok(err instanceof Stripe.errors.StripeIdempotencyError, `expected StripeIdempotencyError, got ${err.type}`);
    const list = await stripe.customers.list({email: 'retry@example.com'});
    assert.equal(list.data[0].id, first.id);
  },

  async 'stripe-connect-transfer'() {
    const account = await stripe.accounts.create({type: 'express', country: 'US', email: 'seller@example.com'});
    assert.equal(account.object, 'account');
    // Transfers draw on the available balance; this test method funds it directly.
    const charge = await stripe.paymentIntents.create({amount: 5000, currency: 'usd', payment_method: 'pm_card_bypassPending', confirm: true, payment_method_types: ['card']});
    assert.equal(charge.status, 'succeeded');
    const transfer = await stripe.transfers.create({amount: 1000, currency: 'usd', destination: account.id});
    assert.equal(transfer.destination, account.id);
    const read = await stripe.transfers.retrieve(transfer.id);
    assert.equal(read.amount, 1000);
    const balance = await stripe.balance.retrieve();
    assert.equal(balance.object, 'balance');
  },

  async 'error-bad-key'() {
    const err = await expectError(() => client({key: 'sk_live_not_a_test_key'}).customers.list());
    assert.ok(err instanceof Stripe.errors.StripeAuthenticationError, `expected StripeAuthenticationError, got ${err.type}`);
    assert.equal(err.statusCode, 401);
  },

  async 'error-missing-resource'() {
    const err = await expectError(() => stripe.customers.retrieve('cus_does_not_exist'));
    assert.ok(err instanceof Stripe.errors.StripeInvalidRequestError);
    assert.equal(err.statusCode, 404);
    assert.equal(err.code, 'resource_missing');
    assert.equal(err.param, 'id');
    assert.ok(err.doc_url);
  },

  async 'error-declined-card'() {
    const err = await expectError(() =>
      stripe.paymentIntents.create({amount: 500, currency: 'usd', confirm: true, payment_method: 'pm_card_visa_chargeDeclinedInsufficientFunds'}),
    );
    assert.ok(err instanceof Stripe.errors.StripeCardError);
    assert.equal(err.decline_code, 'insufficient_funds');
  },

  async 'error-idempotency-mismatch'() {
    const key = `mismatch-${Date.now()}`;
    await stripe.products.create({name: 'A'}, {idempotencyKey: key});
    const err = await expectError(() => stripe.products.create({name: 'B'}, {idempotencyKey: key}));
    assert.ok(err instanceof Stripe.errors.StripeIdempotencyError);
  },

  async 'error-unsupported-version'() {
    const old = client({config: {apiVersion: '2025-09-30.clover'}});
    const err = await expectError(() => old.customers.list());
    assert.ok(err instanceof Stripe.errors.StripeInvalidRequestError);
    assert.equal(err.statusCode, 400);
  },
};

let failed = 0;
for (const [name, fn] of Object.entries(cases)) {
  let pass = true;
  let detail = '';
  try {
    await fn();
  } catch (err) {
    pass = false;
    failed++;
    detail = String(err && err.message ? err.message : err).split('\n')[0];
  }
  console.log(JSON.stringify({sdk, case: name, pass, detail}));
}
process.exit(failed ? 1 : 0);
