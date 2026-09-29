package store

import (
	"crypto/sha1" //nolint:gosec // G505: git object IDs are SHA-1 by definition, not a security control
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Commit is one commit in a repository's history. Files is the full tree at
// that commit, path to blob SHA, so any commit can be read without walking
// history. SHAs are computed the way git computes them, over git's own blob,
// tree and commit encodings, so a blob SHA matches `git hash-object`.
type Commit struct {
	SHA            string            `json:"sha"`
	TreeSHA        string            `json:"tree_sha"`
	Message        string            `json:"message"`
	Parents        []string          `json:"parents"`
	Files          map[string]string `json:"files"`
	AuthorName     string            `json:"author_name"`
	AuthorEmail    string            `json:"author_email"`
	AuthorDate     string            `json:"author_date"`
	CommitterName  string            `json:"committer_name"`
	CommitterEmail string            `json:"committer_email"`
	CommitterDate  string            `json:"committer_date"`
	// Login is the GitHub user the commit is attributed to.
	Login     string `json:"login"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

// Blob is file content addressed by its git SHA.
type Blob struct {
	SHA       string `json:"sha"`
	Content   []byte `json:"content"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

// Errors from the git model.
var (
	ErrNoBranch     = errors.New("branch not found")
	ErrNoCommit     = errors.New("commit not found")
	ErrRefExists    = errors.New("reference already exists")
	ErrNotFastFwd   = errors.New("update is not a fast forward")
	ErrRefNotExists = errors.New("reference does not exist")
)

func objKey(owner, repo, sha string) string     { return RepoKey(owner, repo) + "@" + sha }
func branchKey(owner, repo, name string) string { return RepoKey(owner, repo) + "#" + name }
func refKey(owner, repo, ref string) string     { return RepoKey(owner, repo) + "#" + ref }

func gitHash(kind string, body []byte) (string, []byte) {
	h := sha1.New() //nolint:gosec // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1 -- git object IDs are SHA-1 by definition
	_, _ = fmt.Fprintf(h, "%s %d\x00", kind, len(body))
	h.Write(body)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum), sum
}

// BlobSHA returns the git blob SHA of content.
func BlobSHA(content []byte) string {
	sha, _ := gitHash("blob", content)
	return sha
}

// TreeSHA returns the git tree SHA of a flat path-to-blob map, building the
// nested subtrees git would.
func TreeSHA(files map[string]string) string {
	sha, _ := treeHash(files)
	return sha
}

func treeHash(files map[string]string) (string, []byte) {
	type entry struct {
		name, mode string
		raw        []byte
	}
	dirs := map[string]map[string]string{}
	var entries []entry
	for path, blob := range files {
		if i := strings.IndexByte(path, '/'); i >= 0 {
			d := path[:i]
			if dirs[d] == nil {
				dirs[d] = map[string]string{}
			}
			dirs[d][path[i+1:]] = blob
			continue
		}
		raw, _ := hex.DecodeString(blob)
		entries = append(entries, entry{path, "100644", raw})
	}
	for d, sub := range dirs {
		_, raw := treeHash(sub)
		entries = append(entries, entry{d, "40000", raw})
	}
	// git orders tree entries by name, comparing a directory as name + "/".
	sortName := func(e entry) string {
		if e.mode == "40000" {
			return e.name + "/"
		}
		return e.name
	}
	sort.Slice(entries, func(i, j int) bool { return sortName(entries[i]) < sortName(entries[j]) })
	var body []byte
	for _, e := range entries {
		body = append(body, e.mode+" "+e.name+"\x00"...)
		body = append(body, e.raw...)
	}
	return gitHash("tree", body)
}

func gitTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "0 +0000"
	}
	return fmt.Sprintf("%d +0000", t.Unix())
}

// seal computes the tree and commit SHAs for c.
func seal(c *Commit) {
	c.TreeSHA = TreeSHA(c.Files)
	var b strings.Builder
	fmt.Fprintf(&b, "tree %s\n", c.TreeSHA)
	for _, p := range c.Parents {
		fmt.Fprintf(&b, "parent %s\n", p)
	}
	fmt.Fprintf(&b, "author %s <%s> %s\n", c.AuthorName, c.AuthorEmail, gitTime(c.AuthorDate))
	fmt.Fprintf(&b, "committer %s <%s> %s\n\n%s", c.CommitterName, c.CommitterEmail, gitTime(c.CommitterDate), c.Message)
	c.SHA, _ = gitHash("commit", []byte(b.String()))
}

// PutBlob stores content and returns its SHA.
func (s *MemoryStore) PutBlob(owner, repo string, content []byte) string {
	sha := BlobSHA(content)
	s.Blobs.Set(objKey(owner, repo, sha), Blob{SHA: sha, Content: content, RepoOwner: owner, RepoName: repo})
	return sha
}

// GetBlob returns the content of a blob.
func (s *MemoryStore) GetBlob(owner, repo, sha string) ([]byte, bool) {
	b, ok := s.Blobs.Get(objKey(owner, repo, sha))
	return b.Content, ok
}

// PutCommit seals c, stores it and returns it with its SHAs set.
func (s *MemoryStore) PutCommit(c Commit) Commit {
	if c.Files == nil {
		c.Files = map[string]string{}
	}
	if c.CommitterName == "" {
		c.CommitterName, c.CommitterEmail = "GitHub", "noreply@github.com"
	}
	if c.CommitterDate == "" {
		c.CommitterDate = c.AuthorDate
	}
	seal(&c)
	s.Commits.Set(objKey(c.RepoOwner, c.RepoName, c.SHA), c)
	return c
}

// GetCommit looks a commit up by full SHA, or by an unambiguous prefix of at
// least seven characters, as git accepts.
func (s *MemoryStore) GetCommit(owner, repo, sha string) (Commit, bool) {
	if c, ok := s.Commits.Get(objKey(owner, repo, sha)); ok {
		return c, true
	}
	if len(sha) < 7 || len(sha) >= 40 {
		return Commit{}, false
	}
	matches := s.Commits.Filter(func(_ string, c Commit) bool {
		return c.RepoOwner == owner && c.RepoName == repo && strings.HasPrefix(c.SHA, sha)
	})
	if len(matches) != 1 {
		return Commit{}, false
	}
	return matches[0], true
}

// GetBranch returns a branch of a repository.
func (s *MemoryStore) GetBranch(owner, repo, name string) (Branch, bool) {
	return s.Branches.Get(branchKey(owner, repo, name))
}

// SetBranch points a branch at a commit, creating the branch if needed.
func (s *MemoryStore) SetBranch(owner, repo, name, sha string) {
	b, ok := s.GetBranch(owner, repo, name)
	if !ok {
		b = Branch{Name: name, RepoOwner: owner, RepoName: repo}
	}
	b.Commit.SHA = sha
	s.Branches.Set(branchKey(owner, repo, name), b)
}

// DeleteBranch removes a branch.
func (s *MemoryStore) DeleteBranch(owner, repo, name string) bool {
	return s.Branches.Delete(branchKey(owner, repo, name))
}

// GetTagRef returns a non-branch ref (refs/tags/...).
func (s *MemoryStore) GetTagRef(owner, repo, ref string) (GitRef, bool) {
	return s.GitRefs.Get(refKey(owner, repo, ref))
}

// SetTagRef stores a non-branch ref.
func (s *MemoryStore) SetTagRef(owner, repo, ref, sha string) {
	s.GitRefs.Set(refKey(owner, repo, ref), GitRef{
		Ref: ref, Object: GitObject{Type: "commit", SHA: sha}, RepoOwner: owner, RepoName: repo,
	})
}

// DeleteTagRef removes a non-branch ref.
func (s *MemoryStore) DeleteTagRef(owner, repo, ref string) bool {
	return s.GitRefs.Delete(refKey(owner, repo, ref))
}

// ResolveRef turns what a client passes as a ref (a branch, a tag, a
// refs/... name or a commit SHA) into a commit. An empty ref means the
// repository's default branch.
func (s *MemoryStore) ResolveRef(owner, repo, ref string) (Commit, bool) {
	if ref == "" {
		rp, ok := s.GetRepo(owner, repo)
		if !ok {
			return Commit{}, false
		}
		ref = rp.DefaultBranch
	}
	name := strings.TrimPrefix(ref, "refs/")
	if b, ok := s.GetBranch(owner, repo, strings.TrimPrefix(name, "heads/")); ok {
		return s.GetCommit(owner, repo, b.Commit.SHA)
	}
	if t, ok := s.GetTagRef(owner, repo, "refs/tags/"+strings.TrimPrefix(name, "tags/")); ok {
		return s.GetCommit(owner, repo, t.Object.SHA)
	}
	return s.GetCommit(owner, repo, ref)
}

// IsAncestor reports whether ancestor is reachable from sha by parents.
func (s *MemoryStore) IsAncestor(owner, repo, ancestor, sha string) bool {
	seen := map[string]bool{}
	stack := []string{sha}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == ancestor {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if c, ok := s.GetCommit(owner, repo, cur); ok {
			stack = append(stack, c.Parents...)
		}
	}
	return false
}

// History returns the commits reachable from sha, newest first, stopping at
// (and excluding) anything reachable from stop when stop is set.
func (s *MemoryStore) History(owner, repo, sha, stop string) []Commit {
	excluded := map[string]bool{}
	if stop != "" {
		for _, c := range s.History(owner, repo, stop, "") {
			excluded[c.SHA] = true
		}
	}
	var out []Commit
	seen := map[string]bool{}
	queue := []string{sha}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] || excluded[cur] {
			continue
		}
		seen[cur] = true
		c, ok := s.GetCommit(owner, repo, cur)
		if !ok {
			continue
		}
		out = append(out, c)
		queue = append(queue, c.Parents...)
	}
	return out
}

// CommitChange writes (or, with content nil, deletes) one file on a branch
// and returns the new commit. On an empty repository the branch is created
// with a root commit, which is how GitHub bootstraps a repository through
// the contents API.
func (s *MemoryStore) CommitChange(owner, repo, branch, path string, content []byte, message string, author Signature) (Commit, error) {
	files := map[string]string{}
	var parents []string
	if b, ok := s.GetBranch(owner, repo, branch); ok {
		head, found := s.GetCommit(owner, repo, b.Commit.SHA)
		if !found {
			return Commit{}, ErrNoCommit
		}
		for k, v := range head.Files {
			files[k] = v
		}
		parents = []string{head.SHA}
	} else if s.HasCommits(owner, repo) {
		return Commit{}, ErrNoBranch
	}
	if content == nil {
		delete(files, path)
	} else {
		files[path] = s.PutBlob(owner, repo, content)
	}
	c := s.PutCommit(Commit{
		Message: message, Parents: parents, Files: files,
		AuthorName: author.Name, AuthorEmail: author.Email, AuthorDate: author.Date,
		Login: author.Login, RepoOwner: owner, RepoName: repo,
	})
	s.SetBranch(owner, repo, branch, c.SHA)
	return c, nil
}

// HasCommits reports whether a repository has any history.
func (s *MemoryStore) HasCommits(owner, repo string) bool {
	return len(s.ListRepoBranches(owner, repo)) > 0
}

// Signature names who made a change and when.
type Signature struct {
	Name  string
	Email string
	Date  string
	Login string
}
