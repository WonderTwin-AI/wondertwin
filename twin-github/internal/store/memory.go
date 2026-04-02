package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all GitHub twin state in memory.
type MemoryStore struct {
	Repos        *pkgstate.Store[Repository]
	Issues       *pkgstate.Store[Issue]
	PullRequests *pkgstate.Store[PullRequest]
	Comments     *pkgstate.Store[Comment]
	Labels       *pkgstate.Store[Label]
	Milestones   *pkgstate.Store[Milestone]
	Users        *pkgstate.Store[User]
	Webhooks     *pkgstate.Store[Webhook]
	Statuses     *pkgstate.Store[CommitStatus]
	Releases     *pkgstate.Store[Release]
	Branches     *pkgstate.Store[Branch]
	Reactions    *pkgstate.Store[Reaction]
	Clock        *pkgstate.Clock

	// Monotonic counters for issue/PR numbers per repo
	issueCounter atomic.Int64
	idCounter    atomic.Int64
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Repos:        pkgstate.New[Repository]("repo"),
		Issues:       pkgstate.New[Issue]("issue"),
		PullRequests: pkgstate.New[PullRequest]("pr"),
		Comments:     pkgstate.New[Comment]("comment"),
		Labels:       pkgstate.New[Label]("label"),
		Milestones:   pkgstate.New[Milestone]("milestone"),
		Users:        pkgstate.New[User]("user"),
		Webhooks:     pkgstate.New[Webhook]("hook"),
		Statuses:     pkgstate.New[CommitStatus]("status"),
		Releases:     pkgstate.New[Release]("release"),
		Branches:     pkgstate.New[Branch]("branch"),
		Reactions:    pkgstate.New[Reaction]("reaction"),
		Clock:        pkgstate.NewClock(),
	}
}

// NextID returns a monotonically increasing numeric ID.
func (s *MemoryStore) NextID() int64 {
	return s.idCounter.Add(1)
}

// NextIssueNumber returns the next issue/PR number for a repo.
func (s *MemoryStore) NextIssueNumber() int {
	return int(s.issueCounter.Add(1))
}

// RepoKey builds a lookup key for owner/repo.
func RepoKey(owner, repo string) string {
	return owner + "/" + repo
}

// GetRepo looks up a repository by owner and name.
func (s *MemoryStore) GetRepo(owner, repo string) (*Repository, bool) {
	r, ok := s.Repos.Get(RepoKey(owner, repo))
	if !ok {
		return nil, false
	}
	return &r, true
}

// GetIssue looks up an issue by owner/repo/number.
func (s *MemoryStore) GetIssue(owner, repo string, number int) (*Issue, string, bool) {
	ids, issues := s.Issues.FilterWithIDs(func(id string, iss Issue) bool {
		return iss.RepoOwner == owner && iss.RepoName == repo && iss.Number == number
	})
	if len(ids) == 0 {
		return nil, "", false
	}
	return &issues[0], ids[0], true
}

// ListRepoIssues returns issues for a repo.
func (s *MemoryStore) ListRepoIssues(owner, repo, state string) []Issue {
	return s.Issues.Filter(func(_ string, iss Issue) bool {
		if iss.RepoOwner != owner || iss.RepoName != repo {
			return false
		}
		if state != "" && state != "all" && iss.State != state {
			return false
		}
		return true
	})
}

// GetPR looks up a pull request by owner/repo/number.
func (s *MemoryStore) GetPR(owner, repo string, number int) (*PullRequest, string, bool) {
	ids, prs := s.PullRequests.FilterWithIDs(func(id string, pr PullRequest) bool {
		return pr.RepoOwner == owner && pr.RepoName == repo && pr.Number == number
	})
	if len(ids) == 0 {
		return nil, "", false
	}
	return &prs[0], ids[0], true
}

// ListRepoPRs returns pull requests for a repo.
func (s *MemoryStore) ListRepoPRs(owner, repo, state string) []PullRequest {
	return s.PullRequests.Filter(func(_ string, pr PullRequest) bool {
		if pr.RepoOwner != owner || pr.RepoName != repo {
			return false
		}
		if state != "" && state != "all" && pr.State != state {
			return false
		}
		return true
	})
}

// ListIssueComments returns comments for an issue.
func (s *MemoryStore) ListIssueComments(owner, repo string, issueNumber int) []Comment {
	return s.Comments.Filter(func(_ string, c Comment) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.IssueNumber == issueNumber
	})
}

// ListRepoLabels returns labels for a repo.
func (s *MemoryStore) ListRepoLabels(owner, repo string) []Label {
	return s.Labels.Filter(func(_ string, l Label) bool {
		return l.RepoOwner == owner && l.RepoName == repo
	})
}

// GetLabel looks up a label by owner/repo/name.
func (s *MemoryStore) GetLabel(owner, repo, name string) (*Label, string, bool) {
	ids, labels := s.Labels.FilterWithIDs(func(id string, l Label) bool {
		return l.RepoOwner == owner && l.RepoName == repo && l.Name == name
	})
	if len(ids) == 0 {
		return nil, "", false
	}
	return &labels[0], ids[0], true
}

// ListRepoWebhooks returns webhooks for a repo.
func (s *MemoryStore) ListRepoWebhooks(owner, repo string) []Webhook {
	return s.Webhooks.Filter(func(_ string, w Webhook) bool {
		return w.RepoOwner == owner && w.RepoName == repo
	})
}

// ListRepoStatuses returns statuses for a specific SHA.
func (s *MemoryStore) ListRepoStatuses(owner, repo, sha string) []CommitStatus {
	return s.Statuses.Filter(func(_ string, cs CommitStatus) bool {
		return cs.RepoOwner == owner && cs.RepoName == repo && cs.SHA == sha
	})
}

// ListRepoReleases returns releases for a repo.
func (s *MemoryStore) ListRepoReleases(owner, repo string) []Release {
	return s.Releases.Filter(func(_ string, r Release) bool {
		return r.RepoOwner == owner && r.RepoName == repo
	})
}

// ListRepoBranches returns branches for a repo.
func (s *MemoryStore) ListRepoBranches(owner, repo string) []Branch {
	return s.Branches.Filter(func(_ string, b Branch) bool {
		return b.RepoOwner == owner && b.RepoName == repo
	})
}

// Now returns a formatted GitHub timestamp.
func (s *MemoryStore) Now() string {
	return s.Clock.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// BaseURL returns the simulated GitHub base URL.
func (s *MemoryStore) BaseURL() string {
	return "https://github.com"
}

// APIURL returns the simulated API base URL.
func (s *MemoryStore) APIURL() string {
	return "https://api.github.com"
}

// DefaultSHA returns a deterministic fake SHA.
func DefaultSHA() string {
	return "abc1234567890def1234567890abcdef12345678"
}

// MakeSHA generates a deterministic SHA from a string.
func MakeSHA(input string) string {
	return fmt.Sprintf("%040x", []byte(input))[:40]
}

type stateSnapshot struct {
	Repos        map[string]Repository    `json:"repos,omitempty"`
	Issues       map[string]Issue         `json:"issues,omitempty"`
	PullRequests map[string]PullRequest   `json:"pull_requests,omitempty"`
	Comments     map[string]Comment       `json:"comments,omitempty"`
	Labels       map[string]Label         `json:"labels,omitempty"`
	Milestones   map[string]Milestone     `json:"milestones,omitempty"`
	Users        map[string]User          `json:"users,omitempty"`
	Webhooks     map[string]Webhook       `json:"webhooks,omitempty"`
	Statuses     map[string]CommitStatus  `json:"statuses,omitempty"`
	Releases     map[string]Release       `json:"releases,omitempty"`
	Branches     map[string]Branch        `json:"branches,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Repos:        s.Repos.Snapshot(),
		Issues:       s.Issues.Snapshot(),
		PullRequests: s.PullRequests.Snapshot(),
		Comments:     s.Comments.Snapshot(),
		Labels:       s.Labels.Snapshot(),
		Milestones:   s.Milestones.Snapshot(),
		Users:        s.Users.Snapshot(),
		Webhooks:     s.Webhooks.Snapshot(),
		Statuses:     s.Statuses.Snapshot(),
		Releases:     s.Releases.Snapshot(),
		Branches:     s.Branches.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Repos != nil {
		s.Repos.LoadSnapshot(snap.Repos)
	}
	if snap.Issues != nil {
		s.Issues.LoadSnapshot(snap.Issues)
	}
	if snap.PullRequests != nil {
		s.PullRequests.LoadSnapshot(snap.PullRequests)
	}
	if snap.Comments != nil {
		s.Comments.LoadSnapshot(snap.Comments)
	}
	if snap.Labels != nil {
		s.Labels.LoadSnapshot(snap.Labels)
	}
	if snap.Milestones != nil {
		s.Milestones.LoadSnapshot(snap.Milestones)
	}
	if snap.Users != nil {
		s.Users.LoadSnapshot(snap.Users)
	}
	if snap.Webhooks != nil {
		s.Webhooks.LoadSnapshot(snap.Webhooks)
	}
	if snap.Statuses != nil {
		s.Statuses.LoadSnapshot(snap.Statuses)
	}
	if snap.Releases != nil {
		s.Releases.LoadSnapshot(snap.Releases)
	}
	if snap.Branches != nil {
		s.Branches.LoadSnapshot(snap.Branches)
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Repos.Reset()
	s.Issues.Reset()
	s.PullRequests.Reset()
	s.Comments.Reset()
	s.Labels.Reset()
	s.Milestones.Reset()
	s.Users.Reset()
	s.Webhooks.Reset()
	s.Statuses.Reset()
	s.Releases.Reset()
	s.Branches.Reset()
	s.Reactions.Reset()
	s.Clock.Reset()
	s.issueCounter.Store(0)
	s.idCounter.Store(0)
}
