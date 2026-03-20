package workspace

import "fmt"

// validateWorkflowTransition checks if a status transition is allowed by the workflow.
func validateWorkflowTransition(wf *WorkflowConfig, from, to string) error {
	allowed, ok := wf.Transitions[from]
	if !ok {
		// Check if it's a terminal state.
		for _, ts := range wf.TerminalStates {
			if from == ts {
				return fmt.Errorf("cannot transition from terminal state %q", from)
			}
		}
		// Unknown state with no transitions defined — allow any transition.
		return nil
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("transition from %q to %q is not allowed", from, to)
}
