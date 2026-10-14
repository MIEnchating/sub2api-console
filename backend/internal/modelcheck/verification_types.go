package modelcheck

const verificationVersion = "verification-v1"

// VerificationDimension reports evidence, never a probability of model identity.
type VerificationDimension struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type VerificationEvidence struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type VerificationProbe struct {
	ID              string          `json:"id"`
	Label           string          `json:"label"`
	Round           int             `json:"round"`
	RequestID       string          `json:"request_id"`
	Prompt          string          `json:"prompt"`
	Expected        string          `json:"expected"`
	Answer          string          `json:"answer,omitempty"`
	AnswerTruncated bool            `json:"answer_truncated,omitempty"`
	Status          string          `json:"status"`
	ErrorCode       string          `json:"error_code,omitempty"`
	Error           string          `json:"error,omitempty"`
	ResponseModel   string          `json:"response_model,omitempty"`
	Protocol        string          `json:"protocol,omitempty"`
	HTTPStatus      int             `json:"http_status,omitempty"`
	Attempts        int             `json:"attempts"`
	DurationMS      int64           `json:"duration_ms"`
	Usage           *AnimationUsage `json:"usage,omitempty"`
}

type VerificationReport struct {
	Version      string                 `json:"version"`
	Authenticity VerificationDimension  `json:"authenticity"`
	Capability   VerificationDimension  `json:"capability"`
	Evidence     []VerificationEvidence `json:"evidence"`
	Probes       []VerificationProbe    `json:"probes"`
	Passed       int                    `json:"passed"`
	Total        int                    `json:"total"`
}
