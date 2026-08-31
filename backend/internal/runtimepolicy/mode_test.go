package runtimepolicy

import "testing"

func TestModeCapabilities(t *testing.T) {
	tests := []struct {
		mode string
		want Capabilities
	}{
		{Monitoring, Capabilities{AutomaticUpstreamSync: true, ManualAccountMultiplier: true}},
		{Full, Capabilities{
			PersistRoutingDecisions: true, AutomaticRemoteApply: true, AutomaticUpstreamSync: true,
			AutomaticActiveProbe: true, ManualAccountMultiplier: true, ManualAccountFields: true, RemoteTopologyChanges: true,
		}},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			got, valid := For(test.mode)
			if !valid || got != test.want {
				t.Fatalf("capabilities=%#v valid=%v want=%#v", got, valid, test.want)
			}
		})
	}
	if capabilities, valid := For("配置错误"); valid || capabilities != (Capabilities{}) {
		t.Fatalf("invalid mode capabilities=%#v valid=%v", capabilities, valid)
	}
	if capabilities, valid := For("调度模式"); valid || capabilities != (Capabilities{}) {
		t.Fatalf("removed mode capabilities=%#v valid=%v", capabilities, valid)
	}
}
