package contract

import "errors"

var (
	// ErrContractNotFound is returned by StateStore.Load when no contract
	// with the given ID exists.
	ErrContractNotFound = errors.New("contract: not found")

	// ErrInvalidStatus is returned when an operation is attempted from a
	// status that doesn't permit it (e.g., RecordSignature on a DRAFT).
	ErrInvalidStatus = errors.New("contract: invalid status for operation")

	// ErrInvalidParty is returned when a party reference doesn't match
	// any party on the contract, or when a party's role doesn't permit
	// the action (e.g., RecordSignature on a CC).
	ErrInvalidParty = errors.New("contract: invalid party")

	// ErrPartyNotInActiveSet is returned when RecordSignature targets a
	// party whose RoutingOrder hasn't been reached yet.
	ErrPartyNotInActiveSet = errors.New("contract: party not in active routing-order set")

	// ErrAlreadySigned is returned when RecordSignature targets a party
	// that already has a signature recorded.
	ErrAlreadySigned = errors.New("contract: party already signed")

	// ErrWitnessPairing is returned when a WITNESS party's PairedWith
	// references a non-existent party, a non-SIGNER, or has a
	// RoutingOrder that doesn't match the paired signer.
	ErrWitnessPairing = errors.New("contract: invalid witness pairing")

	// ErrApprovalChainNotImplemented is returned by SubmitForApproval
	// and Approve in v0.1. The approval-chain surface lands in v0.2 and
	// will be shaped by the first CLM consumer.
	ErrApprovalChainNotImplemented = errors.New("contract: approval chain is v0.2 (not yet implemented)")
)
