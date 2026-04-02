// Package store defines the GitHub twin's state types and in-memory store.
package store

// Repository represents a GitHub repository.
type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Language      string `json:"language,omitempty"`
	ForksCount    int    `json:"forks_count"`
	StarCount     int    `json:"stargazers_count"`
	WatcherCount  int    `json:"watchers_count"`
	OpenIssues    int    `json:"open_issues_count"`
	HasIssues     bool   `json:"has_issues"`
	HasProjects   bool   `json:"has_projects"`
	HasWiki       bool   `json:"has_wiki"`
	Archived      bool   `json:"archived"`
	Disabled      bool   `json:"disabled"`
	Owner         User   `json:"owner"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	PushedAt      string `json:"pushed_at"`
	Topics        []string `json:"topics,omitempty"`
}

// Issue represents a GitHub issue.
type Issue struct {
	ID        int64    `json:"id"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"` // "open" or "closed"
	User      User     `json:"user"`
	Labels    []Label  `json:"labels,omitempty"`
	Assignees []User   `json:"assignees,omitempty"`
	Milestone *Milestone `json:"milestone,omitempty"`
	Locked    bool     `json:"locked"`
	Comments  int      `json:"comments"`
	HTMLURL   string   `json:"html_url"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	ClosedAt  string   `json:"closed_at,omitempty"`

	// Internal: which repo this belongs to
	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	ID        int64  `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"` // "open", "closed"
	Draft     bool   `json:"draft"`
	Merged    bool   `json:"merged"`
	User      User   `json:"user"`
	Head      PRRef  `json:"head"`
	Base      PRRef  `json:"base"`
	Labels    []Label `json:"labels,omitempty"`
	Assignees []User  `json:"assignees,omitempty"`
	HTMLURL   string `json:"html_url"`
	DiffURL   string `json:"diff_url"`
	MergeCommitSHA string `json:"merge_commit_sha,omitempty"`
	Mergeable      *bool  `json:"mergeable,omitempty"`
	Comments       int    `json:"comments"`
	Commits        int    `json:"commits"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changed_files"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ClosedAt       string `json:"closed_at,omitempty"`
	MergedAt       string `json:"merged_at,omitempty"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// PRRef represents the head or base of a pull request.
type PRRef struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
}

// Comment represents an issue or PR comment.
type Comment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	User      User   `json:"user"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	RepoOwner   string `json:"-"`
	RepoName    string `json:"-"`
	IssueNumber int    `json:"-"`
}

// Label represents a GitHub label.
type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color"`
	Default     bool   `json:"default"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// Milestone represents a GitHub milestone.
type Milestone struct {
	ID          int64  `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	State       string `json:"state"` // "open" or "closed"
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// User represents a GitHub user (also used as org).
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Type      string `json:"type"` // "User" or "Organization"
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
	SiteAdmin bool   `json:"site_admin"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Bio       string `json:"bio,omitempty"`
	Company   string `json:"company,omitempty"`
	Location  string `json:"location,omitempty"`
}

// Webhook represents a GitHub webhook.
type Webhook struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Active    bool     `json:"active"`
	Events    []string `json:"events"`
	Config    WebhookConfig `json:"config"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// WebhookConfig holds webhook delivery configuration.
type WebhookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret,omitempty"`
	InsecureSSL string `json:"insecure_ssl,omitempty"`
}

// CommitStatus represents a commit status.
type CommitStatus struct {
	ID          int64  `json:"id"`
	State       string `json:"state"` // "error", "failure", "pending", "success"
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context"`
	Creator     User   `json:"creator"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
	SHA       string `json:"-"`
}

// Release represents a GitHub release.
type Release struct {
	ID              int64  `json:"id"`
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
	TargetCommitish string `json:"target_commitish"`
	HTMLURL         string `json:"html_url"`
	Author          User   `json:"author"`
	CreatedAt       string `json:"created_at"`
	PublishedAt     string `json:"published_at,omitempty"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// Branch represents a Git branch.
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    BranchCommit `json:"commit"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// BranchCommit is the head commit of a branch.
type BranchCommit struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// Reaction represents a reaction on an issue/comment.
type Reaction struct {
	ID        int64  `json:"id"`
	User      User   `json:"user"`
	Content   string `json:"content"` // "+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", "eyes"
	CreatedAt string `json:"created_at"`

	// Internal: what this reaction is attached to
	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
	Subject   string `json:"-"` // "issue:3", "comment:42", etc.
}

// PRReview represents a pull request review.
type PRReview struct {
	ID          int64  `json:"id"`
	User        User   `json:"user"`
	Body        string `json:"body"`
	State       string `json:"state"` // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "PENDING", "DISMISSED"
	HTMLURL     string `json:"html_url"`
	CommitID    string `json:"commit_id"`
	SubmittedAt string `json:"submitted_at"`

	RepoOwner  string `json:"-"`
	RepoName   string `json:"-"`
	PRNumber   int    `json:"-"`
}

// PRReviewComment represents an inline/diff comment on a PR.
type PRReviewComment struct {
	ID                int64  `json:"id"`
	Body              string `json:"body"`
	Path              string `json:"path"`
	Position          *int   `json:"position,omitempty"`
	Line              *int   `json:"line,omitempty"`
	Side              string `json:"side,omitempty"`
	CommitID          string `json:"commit_id"`
	OriginalCommitID  string `json:"original_commit_id"`
	DiffHunk          string `json:"diff_hunk"`
	User              User   `json:"user"`
	HTMLURL           string `json:"html_url"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	InReplyToID       *int64 `json:"in_reply_to_id,omitempty"`

	RepoOwner  string `json:"-"`
	RepoName   string `json:"-"`
	PRNumber   int    `json:"-"`
	ReviewID   int64  `json:"-"`
}

// CheckRun represents a GitHub check run.
type CheckRun struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	HeadSHA     string `json:"head_sha"`
	Status      string `json:"status"` // "queued", "in_progress", "completed"
	Conclusion  string `json:"conclusion,omitempty"` // "success", "failure", "neutral", "cancelled", "timed_out", "action_required", "skipped"
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	HTMLURL     string `json:"html_url"`
	DetailsURL  string `json:"details_url,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	Output      *CheckRunOutput `json:"output,omitempty"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// CheckRunOutput holds check run output details.
type CheckRunOutput struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Text        string `json:"text,omitempty"`
}

// CheckSuite represents a GitHub check suite.
type CheckSuite struct {
	ID         int64  `json:"id"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Status     string `json:"status"` // "queued", "in_progress", "completed"
	Conclusion string `json:"conclusion,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// Content represents a file or directory in a repository.
type Content struct {
	Type        string `json:"type"` // "file", "dir", "symlink", "submodule"
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url,omitempty"`
	Content     string `json:"content,omitempty"` // base64-encoded for files
	Encoding    string `json:"encoding,omitempty"` // "base64"

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// Organization represents a GitHub organization.
type Organization struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	HTMLURL     string `json:"html_url"`
	AvatarURL   string `json:"avatar_url"`
	Type        string `json:"type"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Team represents a GitHub team within an organization.
type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Permission  string `json:"permission"` // "pull", "push", "admin"
	Privacy     string `json:"privacy"` // "secret", "closed"
	HTMLURL     string `json:"html_url"`

	OrgLogin string   `json:"-"`
	Members  []string `json:"-"` // user logins
}

// DeployKey represents a deploy key on a repository.
type DeployKey struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	ReadOnly  bool   `json:"read_only"`
	CreatedAt string `json:"created_at"`
	Verified  bool   `json:"verified"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// Deployment represents a GitHub deployment.
type Deployment struct {
	ID          int64  `json:"id"`
	Ref         string `json:"ref"`
	Task        string `json:"task"`
	Environment string `json:"environment"`
	Description string `json:"description,omitempty"`
	Creator     User   `json:"creator"`
	SHA         string `json:"sha"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
}

// DeploymentStatus represents a status for a deployment.
type DeploymentStatus struct {
	ID            int64  `json:"id"`
	State         string `json:"state"` // "error", "failure", "inactive", "in_progress", "queued", "pending", "success"
	Description   string `json:"description,omitempty"`
	Environment   string `json:"environment,omitempty"`
	EnvironmentURL string `json:"environment_url,omitempty"`
	LogURL        string `json:"log_url,omitempty"`
	Creator       User   `json:"creator"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`

	RepoOwner    string `json:"-"`
	RepoName     string `json:"-"`
	DeploymentID int64  `json:"-"`
}

// ReleaseAsset represents an asset attached to a release.
type ReleaseAsset struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Label         string `json:"label,omitempty"`
	ContentType   string `json:"content_type"`
	Size          int    `json:"size"`
	State         string `json:"state"` // "uploaded"
	DownloadCount int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	Uploader      User   `json:"uploader"`

	RepoOwner string `json:"-"`
	RepoName  string `json:"-"`
	ReleaseID int64  `json:"-"`
}
