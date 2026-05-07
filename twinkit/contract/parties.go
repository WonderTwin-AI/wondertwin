package contract

import "fmt"

// validateParties enforces the v0.1 party model rules:
//   - at least one SIGNER
//   - WITNESS parties must have PairedWith referencing an existing
//     SIGNER on the contract
//   - WITNESS RoutingOrder must equal its paired signer's RoutingOrder
//   - PairedWith on non-WITNESS roles is ignored (set to nil)
//
// APPROVER parties may appear; the engine ignores them for v0.1
// transitions (reserved for v0.2).
func validateParties(parties []Party) error {
	if len(parties) == 0 {
		return fmt.Errorf("%w: at least one party required", ErrInvalidParty)
	}
	hasSigner := false
	idIndex := make(map[string]int, len(parties))
	for i, p := range parties {
		if p.ID == "" {
			return fmt.Errorf("%w: party at index %d has empty ID", ErrInvalidParty, i)
		}
		if _, dup := idIndex[p.ID]; dup {
			return fmt.Errorf("%w: duplicate party ID %s", ErrInvalidParty, p.ID)
		}
		idIndex[p.ID] = i
		if p.Role == RoleSigner {
			hasSigner = true
		}
	}
	if !hasSigner {
		return fmt.Errorf("%w: at least one SIGNER required", ErrInvalidParty)
	}
	// Witness pairing.
	for _, p := range parties {
		if p.Role != RoleWitness {
			continue
		}
		if p.PairedWith == nil {
			// Unpaired witness is observer-only — allowed.
			continue
		}
		pairedIdx, ok := idIndex[*p.PairedWith]
		if !ok {
			return fmt.Errorf("%w: witness %s paired with unknown party %s", ErrWitnessPairing, p.ID, *p.PairedWith)
		}
		paired := parties[pairedIdx]
		if paired.Role != RoleSigner {
			return fmt.Errorf("%w: witness %s paired with non-SIGNER %s (role %s)", ErrWitnessPairing, p.ID, paired.ID, paired.Role)
		}
		if p.RoutingOrder != paired.RoutingOrder {
			return fmt.Errorf("%w: witness %s routing_order=%d mismatches paired signer %s routing_order=%d", ErrWitnessPairing, p.ID, p.RoutingOrder, paired.ID, paired.RoutingOrder)
		}
	}
	return nil
}

// findPartyIdx returns the index of the party with the given ID, or -1.
func findPartyIdx(parties []Party, id string) int {
	for i, p := range parties {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// activeRoutingOrder returns the lowest RoutingOrder that has at least
// one unsigned required SIGNER (or paired-required WITNESS). Returns
// (-1, false) if every required party has signed.
func activeRoutingOrder(parties []Party) (int, bool) {
	min := 0
	found := false
	for _, p := range parties {
		if !isRequiredForSigning(p) {
			continue
		}
		if !p.SignedAt.IsZero() {
			continue
		}
		if !found || p.RoutingOrder < min {
			min = p.RoutingOrder
			found = true
		}
	}
	return min, found
}

// partyInActiveSet returns true if the party belongs to the current
// active routing-order group.
func partyInActiveSet(parties []Party, p *Party) bool {
	active, ok := activeRoutingOrder(parties)
	if !ok {
		return false
	}
	return p.RoutingOrder == active
}

// allRequiredSigned returns true when every required signing party
// (SIGNER required==true, plus paired WITNESS) has SignedAt set.
func allRequiredSigned(parties []Party) bool {
	for _, p := range parties {
		if !isRequiredForSigning(p) {
			continue
		}
		if p.SignedAt.IsZero() {
			return false
		}
	}
	return true
}

// isRequiredForSigning returns true if the party must sign for the
// contract to advance to SIGNED. Required SIGNERs and paired WITNESSes
// both count.
func isRequiredForSigning(p Party) bool {
	switch {
	case p.Role == RoleSigner && p.Required:
		return true
	case p.Role == RoleWitness && p.PairedWith != nil:
		// Paired witnesses are always required (they sign in tandem
		// with their signer; the pair counts as one obligation).
		return true
	}
	return false
}

// -- defensive copies -----------------------------------------------------

func cloneParties(in []Party) []Party {
	if in == nil {
		return nil
	}
	out := make([]Party, len(in))
	for i, p := range in {
		out[i] = p
		if p.PairedWith != nil {
			s := *p.PairedWith
			out[i].PairedWith = &s
		}
		if p.Attestation != nil {
			a := *p.Attestation
			out[i].Attestation = &a
		}
		if p.Metadata != nil {
			m := make(map[string]any, len(p.Metadata))
			for k, v := range p.Metadata {
				m[k] = v
			}
			out[i].Metadata = m
		}
	}
	return out
}

func cloneDocuments(in []DocumentRef) []DocumentRef {
	if in == nil {
		return nil
	}
	out := make([]DocumentRef, len(in))
	for i, d := range in {
		out[i] = d
		if d.Metadata != nil {
			m := make(map[string]any, len(d.Metadata))
			for k, v := range d.Metadata {
				m[k] = v
			}
			out[i].Metadata = m
		}
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
