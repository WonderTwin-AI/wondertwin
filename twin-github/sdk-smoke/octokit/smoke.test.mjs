// Drives the GitHub app emulator's MVP use cases through Octokit, GitHub's
// official JavaScript SDK: octokit 5.0.5, which resolves
// @octokit/plugin-rest-endpoint-methods 17.0.0 (pinned by package-lock.json).
// Test-only; run.sh installs it in a scratch copy and runs it against a live
// binary with GITHUB_EMULATOR_URL set.
//
// Octokit sends no X-GitHub-Api-Version header. GitHub serves such requests
// as 2022-11-28; the community app emulator serves them as 2026-03-10.
import { test } from "node:test";
import assert from "node:assert/strict";
import { appendFileSync, readFileSync } from "node:fs";
import { createServer } from "node:http";
import { generateKeyPairSync } from "node:crypto";
import { App, Octokit } from "octokit";
import { verify } from "@octokit/webhooks-methods";

const baseUrl = (process.env.GITHUB_EMULATOR_URL ?? "").replace(/\/$/, "");
if (!baseUrl) throw new Error("GITHUB_EMULATOR_URL is not set");
const API_VERSION = "2026-03-10";
const AUTHOR = "ghp_smokeAuthor000000000000000000000000";
const REVIEWER = "ghp_smokeReviewer0000000000000000000000";

const EmulatorOctokit = Octokit.defaults({ baseUrl });
const octokit = (auth) => new EmulatorOctokit(auth ? { auth } : {});

// The pinned SDK reference is what actually ran.
const pem = JSON.parse(readFileSync(new URL("./node_modules/@octokit/plugin-rest-endpoint-methods/package.json", import.meta.url), "utf8"));
assert.equal(pem.version, "17.0.0", "expected @octokit/plugin-rest-endpoint-methods 17.0.0");

async function admin(method, path, body) {
  const res = await fetch(baseUrl + path, {
    method,
    headers: { "content-type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  return res.json();
}

async function fresh() {
  await admin("POST", "/admin/reset");
  await admin("POST", "/admin/state", {
    users: {
      "smoke-author": { id: 190009001, login: "smoke-author", type: "User" },
      "smoke-reviewer": { id: 190009002, login: "smoke-reviewer", type: "User" },
    },
    tokens: {
      [AUTHOR]: { token: AUTHOR, login: "smoke-author", kind: "user" },
      [REVIEWER]: { token: REVIEWER, login: "smoke-reviewer", kind: "user" },
    },
  });
}

function smoke(name, fn) {
  test(name, async () => {
    let status = "FAIL";
    try {
      await fresh();
      await fn();
      status = "PASS";
    } finally {
      if (process.env.SMOKE_RESULTS) appendFileSync(process.env.SMOKE_RESULTS, `octokit\t${name}\t${status}\n`);
    }
  });
}

async function rejects(promise, status) {
  try {
    await promise;
  } catch (err) {
    assert.equal(err.status, status, `expected ${status}, got ${err.status}: ${err.message}`);
    return err;
  }
  assert.fail(`expected a ${status}`);
}

async function bootstrap(gh, repo) {
  const { data } = await gh.rest.repos.createForAuthenticatedUser({ name: repo });
  const owner = data.owner.login;
  const put = await gh.rest.repos.createOrUpdateFileContents({
    owner, repo, path: "README.md", message: "init", content: Buffer.from("# hello\n").toString("base64"),
  });
  return { owner, sha: put.data.commit.sha };
}

// A GitHub App built the way Octokit builds one. The emulator checks the
// JWT's claims, not its signature, so a throwaway key signs it.
async function emulatorApp() {
  const state = await admin("GET", "/admin/state");
  const appId = Object.values(state.apps)[0].id;
  const { privateKey } = generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  return new App({ appId, privateKey, Octokit: EmulatorOctokit });
}

async function installationOctokit() {
  const app = await emulatorApp();
  const { data: installs } = await app.octokit.rest.apps.listInstallations();
  return app.getInstallationOctokit(installs[0].id);
}

smoke("repo-bootstrap", async () => {
  const gh = octokit(AUTHOR);
  const { data: repo } = await gh.rest.repos.createForAuthenticatedUser({ name: "boot" });
  assert.ok(repo.id > 1_000_000_000 && repo.node_id.startsWith("R_"));
  const put = await gh.rest.repos.createOrUpdateFileContents({
    owner: "smoke-author", repo: "boot", path: "README.md", message: "init",
    content: Buffer.from("# hello\n").toString("base64"),
  });
  assert.equal(put.status, 201);
  const { data: branches } = await gh.rest.repos.listBranches({ owner: "smoke-author", repo: "boot" });
  assert.equal(branches[0].commit.sha, put.data.commit.sha);
  const { data: file } = await gh.rest.repos.getContent({ owner: "smoke-author", repo: "boot", path: "README.md" });
  assert.equal(file.type, "file");
  assert.equal(Buffer.from(file.content, "base64").toString(), "# hello\n");
  assert.equal(file.sha, put.data.content.sha);
  const got = await gh.rest.repos.get({ owner: "smoke-author", repo: "boot" });
  assert.equal(got.data.id, repo.id);
  assert.equal(got.headers["x-github-api-version-selected"], API_VERSION);
  assert.equal(got.data.has_downloads, undefined, "2026-03-10 has no has_downloads");
  await rejects(gh.rest.repos.createForAuthenticatedUser({ name: "boot" }), 422);
});

smoke("issue-triage", async () => {
  const gh = octokit(AUTHOR);
  const { owner } = await bootstrap(gh, "triage");
  await gh.rest.issues.createLabel({ owner, repo: "triage", name: "triage", color: "fbca04" });
  for (let i = 0; i < 3; i++) await gh.rest.issues.create({ owner, repo: "triage", title: `older ${i}` });
  const { data: issue } = await gh.rest.issues.create({ owner, repo: "triage", title: "Build fails on main" });
  assert.equal(issue.assignee, undefined, "2026-03-10 has no singular assignee");
  await gh.rest.issues.addLabels({ owner, repo: "triage", issue_number: issue.number, labels: ["triage"] });
  await gh.rest.issues.createComment({ owner, repo: "triage", issue_number: issue.number, body: "Reproduced." });

  const first = await gh.rest.issues.listForRepo({ owner, repo: "triage", per_page: 2 });
  assert.match(first.headers.link, /rel="next"/);
  const all = await gh.paginate(gh.rest.issues.listForRepo, { owner, repo: "triage", per_page: 2 });
  assert.equal(all.length, 4);

  await gh.rest.issues.update({ owner, repo: "triage", issue_number: issue.number, state: "closed", state_reason: "completed" });
  const { data: got } = await gh.rest.issues.get({ owner, repo: "triage", issue_number: issue.number });
  assert.equal(got.state, "closed");
  assert.equal(got.labels[0].name, "triage");
  assert.equal(got.comments, 1);
});

smoke("pr-lifecycle", async () => {
  const gh = octokit(AUTHOR);
  const { owner } = await bootstrap(gh, "prs");
  const { data: main } = await gh.rest.git.getRef({ owner, repo: "prs", ref: "heads/main" });
  await gh.rest.git.createRef({ owner, repo: "prs", ref: "refs/heads/feature", sha: main.object.sha });
  const put = await gh.rest.repos.createOrUpdateFileContents({
    owner, repo: "prs", path: "CHANGELOG.md", message: "add changelog", branch: "feature",
    content: Buffer.from("# Changelog\n").toString("base64"),
  });
  const { data: pr } = await gh.rest.pulls.create({ owner, repo: "prs", title: "Add changelog", head: "feature", base: "main" });
  assert.equal(pr.head.sha, put.data.commit.sha);
  assert.equal(pr.merge_commit_sha, undefined, "2026-03-10 has no merge_commit_sha");

  await rejects(gh.rest.pulls.createReview({ owner, repo: "prs", pull_number: pr.number, event: "APPROVE" }), 422);
  const { data: review } = await octokit(REVIEWER).rest.pulls.createReview({ owner, repo: "prs", pull_number: pr.number, event: "APPROVE" });
  assert.equal(review.state, "APPROVED");

  const { data: merge } = await gh.rest.pulls.merge({ owner, repo: "prs", pull_number: pr.number, merge_method: "squash" });
  assert.equal(merge.merged, true);
  const check = await gh.rest.pulls.checkIfMerged({ owner, repo: "prs", pull_number: pr.number });
  assert.equal(check.status, 204);
  const { data: got } = await gh.rest.pulls.get({ owner, repo: "prs", pull_number: pr.number });
  assert.equal(got.state, "closed");
  assert.equal(got.merged, true);
});

smoke("ci-status-and-checks", async () => {
  const gh = octokit(AUTHOR);
  const { owner, sha } = await bootstrap(gh, "ci");
  await gh.rest.repos.createCommitStatus({ owner, repo: "ci", sha, state: "success", context: "ci/unit" });
  const { data: combined } = await gh.rest.repos.getCombinedStatusForRef({ owner, repo: "ci", ref: sha });
  assert.equal(combined.state, "success");

  await rejects(gh.rest.checks.create({ owner, repo: "ci", name: "ci/integration", head_sha: sha }), 403);
  const inst = await installationOctokit();
  const { data: run } = await inst.rest.checks.create({ owner, repo: "ci", name: "ci/integration", head_sha: sha, status: "in_progress" });
  await inst.rest.checks.update({ owner, repo: "ci", check_run_id: run.id, status: "completed", conclusion: "success" });
  const { data: list } = await gh.rest.checks.listForRef({ owner, repo: "ci", ref: sha });
  assert.equal(list.total_count, 1);
  assert.equal(list.check_runs[0].conclusion, "success");
});

smoke("app-installation-token", async () => {
  // The seeded App is installed on the default user's account; an
  // unregistered well-formed token acts as that user.
  await bootstrap(octokit("ghp_defaultUser000000000000000000000000"), "app-repo");
  const app = await emulatorApp();
  const { data: me } = await app.octokit.rest.apps.getAuthenticated();
  const { data: installs } = await app.octokit.rest.apps.listInstallations();
  assert.equal(installs[0].app_id, me.id);
  const { data: token } = await app.octokit.rest.apps.createInstallationAccessToken({ installation_id: installs[0].id });
  assert.match(token.token, /^ghs_[A-Za-z0-9]{36}$/);
  const inst = await app.getInstallationOctokit(installs[0].id);
  const { data: repos } = await inst.rest.apps.listReposAccessibleToInstallation();
  assert.ok(repos.total_count >= 1);
});

smoke("release-publish", async () => {
  const gh = octokit(AUTHOR);
  const { owner } = await bootstrap(gh, "rel");
  const { data: rel } = await gh.rest.repos.createRelease({ owner, repo: "rel", tag_name: "v1.0.0", name: "v1.0.0" });
  const { data: latest } = await gh.rest.repos.getLatestRelease({ owner, repo: "rel" });
  const { data: byTag } = await gh.rest.repos.getReleaseByTag({ owner, repo: "rel", tag: "v1.0.0" });
  assert.equal(latest.id, rel.id);
  assert.equal(byTag.id, rel.id);
  // Octokit sends uploads to uploads.github.com unless baseUrl is given, as
  // for GitHub Enterprise Server; the emulator serves them on its own host.
  assert.ok(rel.upload_url.startsWith(baseUrl));
  const { data: asset } = await gh.rest.repos.uploadReleaseAsset({
    owner, repo: "rel", release_id: rel.id, name: "artifact.txt", data: "artifact", baseUrl,
  });
  assert.equal(asset.name, "artifact.txt");
});

smoke("api-version-negotiation", async () => {
  const gh = octokit(AUTHOR);
  const { data: versions } = await gh.rest.meta.getAllVersions();
  assert.deepEqual(versions, [API_VERSION]);
  const { owner } = await bootstrap(gh, "ver");
  const pinned = await gh.request("GET /repos/{owner}/{repo}", { owner, repo: "ver", headers: { "x-github-api-version": API_VERSION } });
  assert.equal(pinned.headers["x-github-api-version-selected"], API_VERSION);
  const { data: limits } = await gh.rest.rateLimit.get();
  assert.equal(limits.rate, undefined, "2026-03-10 has no top-level rate");
  assert.ok(limits.resources.core.limit > 0);
});

smoke("workflow-dispatch", async () => {
  const gh = octokit(AUTHOR);
  const { owner } = await bootstrap(gh, "wf");
  await gh.rest.repos.createOrUpdateFileContents({
    owner, repo: "wf", path: ".github/workflows/deploy.yml", message: "add workflow",
    content: Buffer.from("name: Deploy\non:\n  workflow_dispatch:\n").toString("base64"),
  });
  const res = await gh.rest.actions.createWorkflowDispatch({ owner, repo: "wf", workflow_id: "deploy.yml", ref: "main" });
  assert.equal(res.status, 200, "2026-03-10 answers 200 with run details");
  const { data: run } = await gh.rest.actions.getWorkflowRun({ owner, repo: "wf", run_id: res.data.workflow_run_id });
  assert.equal(run.status, "completed");
  assert.equal(run.conclusion, "success");
  const { data: runs } = await gh.rest.actions.listWorkflowRuns({ owner, repo: "wf", workflow_id: "deploy.yml" });
  assert.equal(runs.total_count, 1);
});

smoke("webhook-delivery", async () => {
  const secret = "smoke-secret";
  const received = [];
  const server = createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", async () => {
      received.push({
        event: req.headers["x-github-event"], delivery: req.headers["x-github-delivery"],
        valid: await verify(secret, body, req.headers["x-hub-signature-256"]), payload: JSON.parse(body),
      });
      res.end();
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  try {
    const gh = octokit(AUTHOR);
    const { owner, sha } = await bootstrap(gh, "hooks");
    await gh.rest.repos.createWebhook({
      owner, repo: "hooks", events: ["issues", "pull_request", "push", "check_run"],
      config: { url: `http://127.0.0.1:${server.address().port}/`, content_type: "json", secret },
    });
    await gh.rest.issues.create({ owner, repo: "hooks", title: "hook me" });
    await gh.rest.git.createRef({ owner, repo: "hooks", ref: "refs/heads/topic", sha });
    await gh.rest.repos.createOrUpdateFileContents({
      owner, repo: "hooks", path: "x.txt", message: "x", branch: "topic", content: Buffer.from("x\n").toString("base64"),
    });
    await gh.rest.pulls.create({ owner, repo: "hooks", title: "t", head: "topic", base: "main" });
    const inst = await installationOctokit();
    await inst.rest.checks.create({ owner, repo: "hooks", name: "c", head_sha: sha, conclusion: "success" });
    await admin("POST", "/admin/webhooks/flush");

    const kinds = received.map((d) => (d.payload.action ? `${d.event}.${d.payload.action}` : d.event));
    for (const want of ["ping", "issues.opened", "push", "pull_request.opened", "check_run.created", "check_run.completed"]) {
      assert.ok(kinds.includes(want), `no ${want} delivery (got ${kinds})`);
    }
    assert.ok(received.every((d) => d.valid), "every X-Hub-Signature-256 verifies with @octokit/webhooks-methods");
    assert.ok(received.every((d) => d.delivery), "every delivery has X-GitHub-Delivery");
  } finally {
    server.close();
  }
});

smoke("error-bad-token", async () => {
  const err = await rejects(octokit("not-a-github-token").rest.users.getAuthenticated(), 401);
  assert.equal(err.response.data.message, "Bad credentials");
});

smoke("error-missing-resource", async () => {
  const err = await rejects(octokit(AUTHOR).rest.repos.get({ owner: "nobody", repo: "nothing" }), 404);
  assert.equal(err.response.data.message, "Not Found");
});

smoke("error-unsupported-version", async () => {
  const gh = octokit(AUTHOR);
  for (const v of ["2022-11-28", "2024-01-01"]) {
    const err = await rejects(gh.request("GET /repos/{owner}/{repo}", { owner: "smoke-author", repo: "x", headers: { "x-github-api-version": v } }), 400);
    assert.match(err.response.data.errors, new RegExp(`"${v}", is not a supported version`));
    assert.match(err.response.data.errors, /"2026-03-10" \(most recent\)/);
  }
});
