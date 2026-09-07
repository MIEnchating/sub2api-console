package business

// AccountRecovery describes the gates evaluated by the routing engine at EvaluatedAt.
// Passing time alone does not constitute a successful recovery or remote write.
type AccountRecovery struct {
	EvaluatedAt string              `json:"evaluated_at"`
	Ready       bool                `json:"ready"`
	Conditions  []RecoveryCondition `json:"conditions"`
}

type RecoveryCondition struct {
	Code   string `json:"code"`
	Met    bool   `json:"met"`
	Detail string `json:"detail"`
}
