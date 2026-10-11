package modelcheck

type TerminalContinuityRound struct {
	Round         int    `json:"round"`
	Verdict       string `json:"verdict"`
	RequestID     string `json:"request_id"`
	Response      string `json:"response,omitempty"`
	ResponseModel string `json:"response_model,omitempty"`
	Error         string `json:"error,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	CompletedAt   string `json:"completed_at"`
}
