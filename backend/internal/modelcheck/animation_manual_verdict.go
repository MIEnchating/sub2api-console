package modelcheck

// AnimationManualVerdict is retained only to read historical task records.
// New conclusions come exclusively from animation quality annotations.
type AnimationManualVerdict struct {
	Verdict   string `json:"verdict"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}
