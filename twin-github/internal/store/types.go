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
	ID      int64  `json:"id"`
	User    User   `json:"user"`
	Content string `json:"content"` // "+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", "eyes"
	CreatedAt string `json:"created_at"`
}
