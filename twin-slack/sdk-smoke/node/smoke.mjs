// SDK smoke suite: runs each MVP use case of the Slack app emulator through the
// official @slack/web-api SDK against a running binary.
//
//   SLACK_EMULATOR_URL=http://localhost:4197 node smoke.mjs
//
// Prints one JSON line per case ({"sdk","case","pass","detail"}) and exits
// non-zero if any case fails. run.sh drives both SDK suites.

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {WebClient, LogLevel} from '@slack/web-api';

const base = (process.env.SLACK_EMULATOR_URL || 'http://localhost:4197').replace(/\/$/, '');
const TOKEN = 'xoxb-sdk-smoke-node';
const pkg = JSON.parse(readFileSync(new URL('./node_modules/@slack/web-api/package.json', import.meta.url)));
const sdk = `@slack/web-api ${pkg.version}`;

const web = new WebClient(TOKEN, {
  slackApiUrl: `${base}/api/`,
  retryConfig: {retries: 0},
  logLevel: LogLevel.ERROR,
});

let counter = 0;
const unique = () => `${Date.now().toString(36)}${counter++}`;

async function newChannel() {
  const res = await web.conversations.create({name: `smoke-${unique()}`});
  assert.equal(res.ok, true);
  return res.channel.id;
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
