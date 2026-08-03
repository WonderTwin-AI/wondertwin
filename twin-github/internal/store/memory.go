package store

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all GitHub twin state in memory.
type MemoryStore struct {
	Repos            *pkgstate.Store[Repository]
	Issues           *pkgstate.Store[Issue]
	PullRequests     *pkgstate.Store[PullRequest]
	Comments         *pkgstate.Store[Comment]
	Labels           *pkgstate.Store[Label]
	Milestones       *pkgstate.Store[Milestone]
	Users            *pkgstate.Store[User]
	Webhooks         *pkgstate.Store[Webhook]
	Statuses         *pkgstate.Store[CommitStatus]
	Releases         *pkgstate.Store[Release]
	Branches         *pkgstate.Store[Branch]
	Reactions        *pkgstate.Store[Reaction]
	PRReviews        *pkgstate.Store[PRReview]
	PRReviewComments *pkgstate.Store[PRReviewComment]
	CheckRuns        *pkgstate.Store[CheckRun]
	CheckSuites      *pkgstate.Store[CheckSuite]
	Contents         *pkgstate.Store[Content]
	Orgs             *pkgstate.Store[Organization]
	Teams            *pkgstate.Store[Team]
	DeployKeys       *pkgstate.Store[DeployKey]
	Deployments      *pkgstate.Store[Deployment]
	DeployStatuses   *pkgstate.Store[DeploymentStatus]
	ReleaseAssets    *pkgstate.Store[ReleaseAsset]
	Workflows        *pkgstate.Store[Workflow]
	WorkflowRuns     *pkgstate.Store[WorkflowRun]
	WorkflowJobs     *pkgstate.Store[WorkflowJob]
	Artifacts        *pkgstate.Store[Artifact]
	Secrets          *pkgstate.Store[Secret]
	GitRefs          *pkgstate.Store[GitRef]
	GitCommits       *pkgstate.Store[GitCommit]
	GitTrees         *pkgstate.Store[GitTree]
	GitBlobs         *pkgstate.Store[GitBlob]
	GitTags          *pkgstate.Store[GitTag]
	Clock            *pkgstate.Clock

	// Monotonic counters
	issueCounter atomic.Int64
	idCounter    atomic.Int64
	runCounter   atomic.Int64
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Repos:            pkgstate.New[Repository]("repo"),
		Issues:           pkgstate.New[Issue]("issue"),
		PullRequests:     pkgstate.New[PullRequest]("pr"),
		Comments:         pkgstate.New[Comment]("comment"),
		Labels:           pkgstate.New[Label]("label"),
		Milestones:       pkgstate.New[Milestone]("milestone"),
		Users:            pkgstate.New[User]("user"),
		Webhooks:         pkgstate.New[Webhook]("hook"),
		Statuses:         pkgstate.New[CommitStatus]("status"),
		Releases:         pkgstate.New[Release]("release"),
		Branches:         pkgstate.New[Branch]("branch"),
		Reactions:        pkgstate.New[Reaction]("reaction"),
		PRReviews:        pkgstate.New[PRReview]("review"),
		PRReviewComments: pkgstate.New[PRReviewComment]("rc"),
		CheckRuns:        pkgstate.New[CheckRun]("cr"),
		CheckSuites:      pkgstate.New[CheckSuite]("cs"),
		Contents:         pkgstate.New[Content]("content"),
		Orgs:             pkgstate.New[Organization]("org"),
		Teams:            pkgstate.New[Team]("team"),
		DeployKeys:       pkgstate.New[DeployKey]("dk"),
		Deployments:      pkgstate.New[Deployment]("deploy"),
		DeployStatuses:   pkgstate.New[DeploymentStatus]("ds"),
		ReleaseAssets:    pkgstate.New[ReleaseAsset]("asset"),
		Workflows:        pkgstate.New[Workflow]("wf"),
		WorkflowRuns:     pkgstate.New[WorkflowRun]("run"),
		WorkflowJobs:     pkgstate.New[WorkflowJob]("job"),
		Artifacts:        pkgstate.New[Artifact]("art"),
		Secrets:          pkgstate.New[Secret]("secret"),
		GitRefs:          pkgstate.New[GitRef]("ref"),
		GitCommits:       pkgstate.New[GitCommit]("gc"),
		GitTrees:         pkgstate.New[GitTree]("gt"),
		GitBlobs:         pkgstate.New[GitBlob]("gb"),
		GitTags:          pkgstate.New[GitTag]("gtag"),
		Clock:            pkgstate.NewClock(),
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
	Repos        map[string]Repository   `json:"repos,omitempty"`
	Issues       map[string]Issue        `json:"issues,omitempty"`
	PullRequests map[string]PullRequest  `json:"pull_requests,omitempty"`
	Comments     map[string]Comment      `json:"comments,omitempty"`
	Labels       map[string]Label        `json:"labels,omitempty"`
	Milestones   map[string]Milestone    `json:"milestones,omitempty"`
	Users        map[string]User         `json:"users,omitempty"`
	Webhooks     map[string]Webhook      `json:"webhooks,omitempty"`
	Statuses     map[string]CommitStatus `json:"statuses,omitempty"`
	Releases     map[string]Release      `json:"releases,omitempty"`
	Branches     map[string]Branch       `json:"branches,omitempty"`
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

// ListPRReviews returns reviews for a pull request.
func (s *MemoryStore) ListPRReviews(owner, repo string, prNumber int) []PRReview {
	return s.PRReviews.Filter(func(_ string, r PRReview) bool {
		return r.RepoOwner == owner && r.RepoName == repo && r.PRNumber == prNumber
	})
}

// ListCheckRunsForRef returns check runs for a commit SHA.
func (s *MemoryStore) ListCheckRunsForRef(owner, repo, sha string) []CheckRun {
	return s.CheckRuns.Filter(func(_ string, cr CheckRun) bool {
		return cr.RepoOwner == owner && cr.RepoName == repo && cr.HeadSHA == sha
	})
}

// ListRepoContents returns contents at a path.
func (s *MemoryStore) ListRepoContents(owner, repo, path string) []Content {
	return s.Contents.Filter(func(_ string, c Content) bool {
		return c.RepoOwner == owner && c.RepoName == repo && c.Path == path
	})
}

// ListOrgTeams returns teams for an org.
func (s *MemoryStore) ListOrgTeams(orgLogin string) []Team {
	return s.Teams.Filter(func(_ string, t Team) bool {
		return t.OrgLogin == orgLogin
	})
}

// ListRepoDeployKeys returns deploy keys for a repo.
func (s *MemoryStore) ListRepoDeployKeys(owner, repo string) []DeployKey {
	return s.DeployKeys.Filter(func(_ string, dk DeployKey) bool {
		return dk.RepoOwner == owner && dk.RepoName == repo
	})
}

// ListRepoDeployments returns deployments for a repo.
func (s *MemoryStore) ListRepoDeployments(owner, repo string) []Deployment {
	return s.Deployments.Filter(func(_ string, d Deployment) bool {
		return d.RepoOwner == owner && d.RepoName == repo
	})
}

// ListDeploymentStatuses returns statuses for a deployment.
func (s *MemoryStore) ListDeploymentStatuses(owner, repo string, deployID int64) []DeploymentStatus {
	return s.DeployStatuses.Filter(func(_ string, ds DeploymentStatus) bool {
		return ds.RepoOwner == owner && ds.RepoName == repo && ds.DeploymentID == deployID
	})
}

// ListReleaseAssets returns assets for a release.
func (s *MemoryStore) ListReleaseAssets(owner, repo string, releaseID int64) []ReleaseAsset {
	return s.ReleaseAssets.Filter(func(_ string, a ReleaseAsset) bool {
		return a.RepoOwner == owner && a.RepoName == repo && a.ReleaseID == releaseID
	})
}

// ListRepoMilestones returns milestones for a repo.
func (s *MemoryStore) ListRepoMilestones(owner, repo string) []Milestone {
	return s.Milestones.Filter(func(_ string, m Milestone) bool {
		return m.RepoOwner == owner && m.RepoName == repo
	})
}

// ListSubjectReactions returns reactions for a subject key (e.g., "issue:3").
func (s *MemoryStore) ListSubjectReactions(owner, repo, subject string) []Reaction {
	return s.Reactions.Filter(func(_ string, r Reaction) bool {
		return r.RepoOwner == owner && r.RepoName == repo && r.Subject == subject
	})
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
	s.PRReviews.Reset()
	s.PRReviewComments.Reset()
	s.CheckRuns.Reset()
	s.CheckSuites.Reset()
	s.Contents.Reset()
	s.Orgs.Reset()
	s.Teams.Reset()
	s.DeployKeys.Reset()
	s.Deployments.Reset()
	s.DeployStatuses.Reset()
	s.ReleaseAssets.Reset()
	s.Workflows.Reset()
	s.WorkflowRuns.Reset()
	s.WorkflowJobs.Reset()
	s.Artifacts.Reset()
	s.Secrets.Reset()
	s.GitRefs.Reset()
	s.GitCommits.Reset()
	s.GitTrees.Reset()
	s.GitBlobs.Reset()
	s.GitTags.Reset()
	s.Clock.Reset()
	s.issueCounter.Store(0)
	s.idCounter.Store(0)
	s.runCounter.Store(0)
}

// NextRunNumber returns the next workflow run number.
func (s *MemoryStore) NextRunNumber() int {
	return int(s.runCounter.Add(1))
}

// ListRepoWorkflows returns workflows for a repo.
func (s *MemoryStore) ListRepoWorkflows(owner, repo string) []Workflow {
	return s.Workflows.Filter(func(_ string, w Workflow) bool {
		return w.RepoOwner == owner && w.RepoName == repo
	})
}

// ListWorkflowRuns returns runs for a repo, optionally filtered by workflow ID.
func (s *MemoryStore) ListWorkflowRuns(owner, repo string, workflowID int64) []WorkflowRun {
	return s.WorkflowRuns.Filter(func(_ string, r WorkflowRun) bool {
		if r.RepoOwner != owner || r.RepoName != repo {
			return false
		}
		if workflowID > 0 && r.WorkflowID != workflowID {
			return false
		}
		return true
	})
}

// ListRunJobs returns jobs for a workflow run.
func (s *MemoryStore) ListRunJobs(owner, repo string, runID int64) []WorkflowJob {
	return s.WorkflowJobs.Filter(func(_ string, j WorkflowJob) bool {
		return j.RepoOwner == owner && j.RepoName == repo && j.RunID == runID
	})
}

// ListRunArtifacts returns artifacts for a workflow run.
func (s *MemoryStore) ListRunArtifacts(owner, repo string, runID int64) []Artifact {
	return s.Artifacts.Filter(func(_ string, a Artifact) bool {
		return a.RepoOwner == owner && a.RepoName == repo && a.RunID == runID
	})
}

// ListRepoArtifacts returns all artifacts for a repo.
func (s *MemoryStore) ListRepoArtifacts(owner, repo string) []Artifact {
	return s.Artifacts.Filter(func(_ string, a Artifact) bool {
		return a.RepoOwner == owner && a.RepoName == repo
	})
}

// ListRepoSecrets returns secrets for a repo (metadata only).
func (s *MemoryStore) ListRepoSecrets(owner, repo string) []Secret {
	return s.Secrets.Filter(func(_ string, sec Secret) bool {
		return sec.RepoOwner == owner && sec.RepoName == repo
	})
}

// ListRepoGitRefs returns git refs for a repo.
func (s *MemoryStore) ListRepoGitRefs(owner, repo, prefix string) []GitRef {
	return s.GitRefs.Filter(func(_ string, r GitRef) bool {
		if r.RepoOwner != owner || r.RepoName != repo {
			return false
		}
		if prefix != "" {
			return len(r.Ref) >= len(prefix) && r.Ref[:len(prefix)] == prefix
		}
		return true
	})
}
