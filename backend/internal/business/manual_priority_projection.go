package business

// Manual ownership is authoritative even when a management sync or an older
// scheduler run left automatic states behind. Keep evidence and upstream errors,
// but do not present automatic decisions as current manual-account controls.
func applyManualPriorityProjection(item *accountProjection) {
	item.Health = AccountStateManualPriority
	if item.RoutingState != nil && *item.RoutingState == "cost_blocked" && item.Schedulable != nil && !*item.Schedulable {
		item.Health = "cost_blocked"
	}
	if item.Paused != nil && *item.Paused {
		item.Health = AccountStatePaused
	} else if projectionMetadataState(item) == AccountStateDisabled {
		item.Health = AccountStateDisabled
	}
	item.DesiredHealth = nil
	item.DecisionState = nil
	item.DecisionReason = nil
	if item.manualFused {
		item.DecisionState = stringPointer(AccountStateFused)
		item.DecisionReason = stringPointer("人工熔断，等待手动解除")
		if item.Schedulable != nil && !*item.Schedulable && item.Health == AccountStateManualPriority {
			item.Health = AccountStateFused
		}
	}
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
