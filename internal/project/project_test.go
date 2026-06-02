package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFind_NoFileReturnsNilNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Mark as repo root so Find stops here.
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir"), 0o600); err != nil {
		t.Fatalf("seed .git: %v", err)
	}
	p, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p != nil {
		t.Errorf("want nil project, got %+v", p)
	}
}

func TestFind_LoadsFromCurrentDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteProject(t, dir, `{"account_id": "org_acme"}`)
	p, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p == nil || p.AccountID != "org_acme" {
		t.Errorf("project: got %+v", p)
	}
	if !strings.HasSuffix(p.SourcePath, Filename) {
		t.Errorf("SourcePath should point at the file, got %s", p.SourcePath)
	}
}

func TestFind_WalksUpToParentDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteProject(t, root, `{"account_id": "org_root"}`)
	sub := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	p, err := Find(sub)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p == nil || p.AccountID != "org_root" {
		t.Errorf("walk-up: got %+v", p)
	}
}

func TestFind_StopsAtGitRepoRoot(t *testing.T) {
	t.Parallel()
	// project.json above the repo root must NOT be picked up — the
	// .git boundary stops the walk.
	outside := t.TempDir()
	mustWriteProject(t, outside, `{"account_id": "org_outside"}`)
	repo := filepath.Join(outside, "myrepo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	p, err := Find(repo)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p != nil {
		t.Errorf("walked past .git boundary; got %+v", p)
	}
}

func TestFind_RepoRootProjectStillLoads(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	mustWriteProject(t, repo, `{"account_id": "org_repo"}`)
	p, err := Find(repo)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p == nil || p.AccountID != "org_repo" {
		t.Errorf("want project at repo root, got %+v", p)
	}
}

func TestLoadFile_MissingReturnsNilNil(t *testing.T) {
	t.Parallel()
	p, err := LoadFile(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Errorf("missing file: want nil err, got %v", err)
	}
	if p != nil {
		t.Errorf("missing file: want nil project, got %+v", p)
	}
}

func TestLoadFile_MalformedJSONErrors(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("want error on malformed JSON")
	}
}

func TestLoadFile_MissingAccountIDErrors(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte(`{"unrelated": "field"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "account_id") {
		t.Errorf("want account_id required error, got %v", err)
	}
}

func mustWriteProject(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".wondertwin"), 0o755); err != nil {
		t.Fatalf("mkdir .wondertwin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(body), 0o600); err != nil {
		t.Fatalf("write project: %v", err)
	}
}
