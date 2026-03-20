package quirks

import "github.com/wondertwin-ai/wondertwin/twinkit/admin"

// adminAdapter wraps the quirks Engine to satisfy admin.QuirkStore.
type adminAdapter struct {
	engine *Engine
}

// AdminAdapter returns an admin.QuirkStore that delegates to this engine.
func (e *Engine) AdminAdapter() admin.QuirkStore {
	return &adminAdapter{engine: e}
}

func (a *adminAdapter) ListQuirks() []admin.QuirkStatus {
	qs := a.engine.ListQuirks()
	result := make([]admin.QuirkStatus, len(qs))
	for i, q := range qs {
		result[i] = admin.QuirkStatus{
			ID:      q.ID,
			Summary: q.Summary,
			Enabled: q.Enabled,
		}
	}
	return result
}

func (a *adminAdapter) EnableQuirk(id string) error  { return a.engine.EnableQuirk(id) }
func (a *adminAdapter) DisableQuirk(id string) error { return a.engine.DisableQuirk(id) }
func (a *adminAdapter) IsEnabled(id string) bool     { return a.engine.IsEnabled(id) }
