// SDK smoke suite: runs each MVP use case of the Slack app emulator through the
// official @slack/web-api SDK against a running binary.
//
//   SLACK_EMULATOR_URL=http://localhost:4197 node smoke.mjs
//
// Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
// non-zero if any case fails. run.sh drives both SDK suites.

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {WebClient, LogLevel, ErrorCode} from '@slack/web-api';

const base = (process.env.SLACK_EMULATOR_URL || 'http://localhost:4197').replace(/\/$/, '');
const TOKEN = 'xoxb-sdk-smoke-node';
const pkg = JSON.parse(readFileSync(new URL('./node_modules/@slack/web-api/package.json', import.meta.url)));
const sdk = `@slack/web-api ${pkg.version}`;

const web = new WebClient(TOKEN, {
  slackApiUrl: `${base}/api/`,
  retryConfig: {retries: 0},
  logLevel: LogLevel.ERROR,
});

function client(token) {
  return new WebClient(token, {slackApiUrl: `${base}/api/`, retryConfig: {retries: 0}, logLevel: LogLevel.ERROR});
}

let counter = 0;
const unique = () => `${Date.now().toString(36)}${counter++}`;

async function newChannel() {
  const res = await web.conversations.create({name: `smoke-${unique()}`});
  assert.equal(res.ok, true);
  return res.channel.id;
}


async function platformError(fn) {
  try {
    await fn();
  } catch (err) {
    assert.equal(err.code, ErrorCode.PlatformError, `expected a platform error, got ${err.code}: ${err.message}`);
    return err.data.error;
  }
  assert.fail('expected the call to fail');
}

const cases = {
  async 'slack-bot-post-and-thread'() {
    const who = await web.auth.test();
    assert.equal(who.ok, true);
    const channel = await newChannel();
    const parent = await web.chat.postMessage({channel, text: 'build finished'});
    assert.equal(parent.ok, true);
    assert.ok(parent.ts);
    const reply = await web.chat.postMessage({channel, text: 'logs attached', thread_ts: parent.ts});
    assert.ok(reply.ts);
    const thread = await web.conversations.replies({channel, ts: parent.ts});
    const inThread = thread.messages.find((m) => m.ts === reply.ts);
    assert.ok(inThread, 'the reply appears in conversations.replies for the parent ts');
    assert.equal(inThread.thread_ts, parent.ts);
  },

  async 'slack-read-history-paginated'() {
    const channel = await newChannel();
    for (let i = 1; i <= 5; i++) {
      await web.chat.postMessage({channel, text: `m${i}`});
    }
    // The SDK's own paginator follows response_metadata.next_cursor to the end.
    const found = [];
    let pages = 0;
    for await (const page of web.paginate('conversations.history', {channel, limit: 2})) {
      pages++;
      for (const m of page.messages) found.push(m.text);
    }
    assert.deepEqual(found, ['m5', 'm4', 'm3', 'm2', 'm1']);
    assert.equal(pages, 3);
  },

  async 'slack-oauth-v2-install'() {
    // The install's last step: the app, which has no token yet, exchanges the
    // code from the authorize redirect, then uses the bot token it got.
    const install = await client(undefined).oauth.v2.access({client_id: '123.456', client_secret: 'smoke-secret', code: `code-${unique()}`});
    assert.equal(install.ok, true);
    assert.equal(install.token_type, 'bot');
    assert.ok(install.access_token.startsWith('xoxb-'), install.access_token);
    assert.ok(install.scope, 'the granted scopes are returned');
    const who = await client(install.access_token).auth.test();
    assert.equal(who.ok, true);
    assert.equal(who.team_id, install.team.id);
  },

  async 'slack-upload-file-external'() {
    // files.uploadV2 drives the three-step external upload: get an upload URL,
    // POST the bytes to it, then complete the upload into a channel.
    const channel = await newChannel();
    const content = 'release notes for the smoke test\n';
    const res = await web.files.uploadV2({channel_id: channel, file: Buffer.from(content), filename: 'notes.txt', title: 'Notes'});
    assert.equal(res.ok, true);
    const id = res.files[0].files[0].id;
    const info = await web.files.info({file: id});
    assert.equal(info.file.id, id);
    assert.ok(info.file.channels.includes(channel), 'the file is shared to the channel');
    assert.equal(info.file.size, Buffer.byteLength(content));
  },

  async 'slack-app-home-publish'() {
    // The app publishes a Home tab for the user who opened it. The SDK sends
    // the view as JSON text in a form field.
    const {user_id} = await client('xoxp-sdk-smoke-node-home').auth.test();
    const external_id = `home-${unique()}`;
    const view = {type: 'home', external_id, blocks: [{type: 'section', text: {type: 'mrkdwn', text: 'Welcome'}}]};
    const first = await web.views.publish({user_id, view});
    assert.equal(first.ok, true);
    assert.ok(first.view.id, 'the published view has an id');
    assert.equal(first.view.type, 'home');
    const again = await web.views.publish({user_id, view, hash: first.view.hash});
    assert.equal(again.view.id, first.view.id, 'a user has one Home view');
    const updated = await web.views.update({external_id, view: {...view, callback_id: 'home_v2'}});
    assert.equal(updated.view.id, first.view.id);
    assert.equal(updated.view.callback_id, 'home_v2');
    assert.equal(await platformError(() => web.views.publish({user_id, view, hash: first.view.hash})), 'hash_conflict');
  },

  async 'slack-dm-user'() {
    // Fixture: a workspace user to find. Seeding is the emulator's admin API,
    // not the SDK's; everything after it goes through the SDK.
    const email = `dm-${unique()}@example.com`;
    const seeded = await fetch(`${base}/admin/state`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({users: {U_SMOKE_NODE: {id: 'U_SMOKE_NODE', name: 'smoke-node', profile: {email}}}}),
    });
    assert.equal(seeded.status, 200);
    const found = await web.users.lookupByEmail({email});
    const opened = await web.conversations.open({users: found.user.id});
    assert.equal(opened.ok, true);
    const channel = opened.channel.id;
    const again = await web.conversations.open({users: found.user.id});
    assert.equal(again.channel.id, channel, 'opening again resumes the same DM');
    assert.equal(again.already_open, true);
    const posted = await web.chat.postMessage({channel, text: 'hello there'});
    const history = await web.conversations.history({channel});
    assert.ok(history.messages.some((m) => m.ts === posted.ts && m.text === 'hello there'));
    },

  async 'slack-react-pin-bookmark'() {
    const channel = await newChannel();
    const {ts} = await web.chat.postMessage({channel, text: 'ship it'});
    await web.reactions.add({channel, timestamp: ts, name: 'rocket'});
    const got = await web.reactions.get({channel, timestamp: ts});
    assert.ok(got.message.reactions.some((r) => r.name === 'rocket' && r.count === 1));
    await web.pins.add({channel, timestamp: ts});
    assert.equal(await platformError(() => web.pins.add({channel, timestamp: ts})), 'already_pinned');
    const pins = await web.pins.list({channel});
    assert.equal(pins.items.filter((i) => i.message.ts === ts).length, 1, 'the message is pinned once');
    assert.deepEqual((await web.bookmarks.list({channel_id: channel})).bookmarks, []);
    await web.bookmarks.add({channel_id: channel, title: 'Runbook', type: 'link', link: 'https://example.com/runbook'});
    const bookmarks = await web.bookmarks.list({channel_id: channel});
    assert.ok(bookmarks.bookmarks.some((b) => b.link === 'https://example.com/runbook'));
  },

  async 'slack-message-lifecycle'() {
    const channel = await newChannel();
    const posted = await web.chat.postMessage({channel, text: 'draft'});
    await web.chat.update({channel, ts: posted.ts, text: 'final'});
    let history = await web.conversations.history({channel});
    assert.equal(history.messages.find((m) => m.ts === posted.ts).text, 'final');
    const link = await web.chat.getPermalink({channel, message_ts: posted.ts});
    assert.ok(link.permalink.includes(posted.ts.replace('.', '')) || link.permalink.includes(posted.ts));
    await web.chat.delete({channel, ts: posted.ts});
    history = await web.conversations.history({channel});
    assert.equal(history.messages.find((m) => m.ts === posted.ts), undefined);
  },

  async 'error-unknown-method'() {
    // channels.list was retired by Slack in 2021. Slack answers it, and a name
    // that never existed, with unknown_method.
    assert.equal(await platformError(() => web.apiCall('channels.list')), 'unknown_method');
    assert.equal(await platformError(() => web.apiCall('definitely.notAMethod')), 'unknown_method');
  },

  async 'error-channel-not-found'() {
    assert.equal(await platformError(() => web.chat.postMessage({channel: 'C0NOSUCH', text: 'x'})), 'channel_not_found');
  },

  async 'slack-auth-test-reflects-token'() {
    const bot = await web.auth.test();
    assert.ok(bot.bot_id, 'a bot token carries a bot_id');
    const userClient = client('xoxp-sdk-smoke-node-user');
    const user = await userClient.auth.test();
    assert.ok(!user.bot_id, 'a user token has no bot_id');
    assert.notEqual(user.user_id, bot.user_id);
    assert.equal(user.team_id, bot.team_id);
  },

  async 'slack-api-test'() {
    const res = await new WebClient(undefined, {slackApiUrl: `${base}/api/`, retryConfig: {retries: 0}, logLevel: LogLevel.ERROR}).api.test({foo: 'bar'});
    assert.equal(res.ok, true);
    assert.equal(res.args.foo, 'bar');
    assert.equal(await platformError(() => web.api.test({error: 'my_error'})), 'my_error');
  },

  async 'slack-auth-revoke'() {
    const doomed = client(`xoxb-sdk-smoke-node-revoke-${unique()}`);
    assert.equal((await doomed.auth.revoke()).revoked, true);
    assert.equal(await platformError(() => doomed.auth.test()), 'token_revoked');
    assert.equal((await web.auth.test()).ok, true);
  },

  async 'error-not-authed'() {
    const anonymous = client(undefined);
    assert.equal(await platformError(() => anonymous.auth.test()), 'not_authed');
  },

  async 'error-invalid-auth'() {
    assert.equal(await platformError(() => client('not-a-slack-token').auth.test()), 'invalid_auth');
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
