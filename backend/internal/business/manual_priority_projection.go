package business

// Manual ownership is authoritative even when a management sync or an older
// scheduler run left automatic states behind. Keep evidence and upstream errors,
// but do not present automatic decisions as current manual-account controls.
func applyManualPriorityProjection(item *accountProjection) {
	item.Health = AccountStateManualPriority
	if item.Paused != nil && *item.Paused {
		item.Health = AccountStatePaused
	} else if accountMetadataState(item.metadataRaw) == AccountStateDisabled {
		item.Health = AccountStateDisabled
	}
	item.DesiredHealth = nil
	item.DecisionState = nil
	item.DecisionReason = nil
	item.EvidencePending = false
	item.Recovery = nil
	item.ApplyPending = false
	item.ApplyError = nil
	item.TargetPriority = nil
	item.TargetLoadFactor = nil
	item.TargetSchedulable = nil
	item.TargetConcurrency = nil
	item.Weight = nil
}
