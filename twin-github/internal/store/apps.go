package store

import "fmt"

// App is a GitHub App registration.
type App struct {
	ID          int64             `json:"id"`
	Slug        string            `json:"slug"`
	ClientID    string            `json:"client_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	OwnerLogin  string            `json:"owner_login"`
	Permissions map[string]string `json:"permissions"`
	Events      []string          `json:"events"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// BotLogin is the login an app acts as when it uses an installation token.
func (a App) BotLogin() string { return a.Slug + "[bot]" }

// Installation is an App installed on an account.
type Installation struct {
	ID           int64  `json:"id"`
	AppID        int64  `json:"app_id"`
	AccountLogin string `json:"account_login"`
	// TargetType is "User" or "Organization".
	TargetType string `json:"target_type"`
	// RepositorySelection is "all" or "selected"; Repositories names the
	// selected ones.
	RepositorySelection string            `json:"repository_selection"`
	Repositories        []string          `json:"repositories,omitempty"`
	Permissions         map[string]string `json:"permissions"`
	Events              []string          `json:"events"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

// DefaultAppSlug names the GitHub App every fresh emulator starts with.
const DefaultAppSlug = "wondertwin-app"

// seedApp registers the default App, installed on the default user's
// account with access to all of its repositories, plus its bot user.
func (s *MemoryStore) seedApp() {
	if len(s.Apps.List()) > 0 {
		return
	}
	now := "2026-01-01T00:00:00Z"
	perms := map[string]string{
		"actions": "write", "checks": "write", "contents": "write", "issues": "write",
		"metadata": "read", "pull_requests": "write", "statuses": "write",
	}
	events := []string{"check_run", "check_suite", "issues", "pull_request", "push"}
	app := App{
		ID: s.NewID(KindApp), Slug: DefaultAppSlug, Name: "WonderTwin App",
		Description: "The GitHub App the app emulator is seeded with.",
		OwnerLogin:  DefaultLogin, Permissions: perms, Events: events, CreatedAt: now, UpdatedAt: now,
	}
	app.ClientID = fmt.Sprintf("Iv23li%014x", app.ID)
	s.Apps.Set(fmt.Sprint(app.ID), app)
	inst := Installation{
		ID: s.NewID(KindInstallation), AppID: app.ID, AccountLogin: DefaultLogin, TargetType: "User",
		RepositorySelection: "all", Permissions: perms, Events: events, CreatedAt: now, UpdatedAt: now,
	}
	s.Installations.Set(fmt.Sprint(inst.ID), inst)
}

// ensureBots gives every app a bot user to attribute its writes to.
func (s *MemoryStore) ensureBots() {
	for _, a := range s.Apps.List() {
		if _, ok := s.Users.Get(a.BotLogin()); !ok {
			s.Users.Set(a.BotLogin(), User{ID: s.NewID(KindUser), Login: a.BotLogin(), Type: "Bot"})
		}
	}
}

// GetApp returns an app by ID.
func (s *MemoryStore) GetApp(id int64) (App, bool) { return s.Apps.Get(fmt.Sprint(id)) }

// GetInstallation returns an installation by ID.
func (s *MemoryStore) GetInstallation(id int64) (Installation, bool) {
	return s.Installations.Get(fmt.Sprint(id))
}

// InstallationRepos lists the repositories an installation can reach.
func (s *MemoryStore) InstallationRepos(inst Installation) []Repository {
	return s.Repos.Filter(func(_ string, rp Repository) bool {
		if rp.Owner.Login != inst.AccountLogin {
			return false
		}
		if inst.RepositorySelection != "selected" {
			return true
		}
		for _, n := range inst.Repositories {
			if n == rp.Name {
				return true
			}
		}
		return false
	})
}
