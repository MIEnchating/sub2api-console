package runtimepolicy

const (
	Monitoring = "监控模式"
	Full       = "完全模式"
)

type Capabilities struct {
	PersistRoutingDecisions bool
	AutomaticRemoteApply    bool
	AutomaticUpstreamSync   bool
	AutomaticActiveProbe    bool
	ManualAccountFields     bool
	RemoteTopologyChanges   bool
}

func For(mode string) (Capabilities, bool) {
	switch mode {
	case Monitoring:
		return Capabilities{
			AutomaticUpstreamSync: true,
		}, true
	case Full:
		return Capabilities{
			PersistRoutingDecisions: true,
			AutomaticRemoteApply:    true,
			AutomaticUpstreamSync:   true,
			AutomaticActiveProbe:    true,
			ManualAccountFields:     true,
			RemoteTopologyChanges:   true,
		}, true
	default:
		return Capabilities{}, false
	}
}

func Valid(mode string) bool {
	_, valid := For(mode)
	return valid
}
