package animationquality

// Recheck is independent of human training samples and generation progress.
type Recheck struct {
	Version      int         `json:"version"`
	TaskID       string      `json:"task_id"`
	SourceSHA256 string      `json:"source_sha256"`
	PromptSHA256 string      `json:"prompt_sha256"`
	Prediction   *Prediction `json:"prediction,omitempty"`
}
