package modelcheck

const verificationVersion = "verification-v4"

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
	Category        string          `json:"category,omitempty"`
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
	StartedAt       string          `json:"started_at,omitempty"`
	Usage           *AnimationUsage `json:"usage,omitempty"`
}

type VerificationReport struct {
	CapabilityTotal int                     `json:"capability_total"`
	Fingerprint     VerificationFingerprint `json:"fingerprint"`
	Version         string                  `json:"version"`
	Authenticity    VerificationDimension   `json:"authenticity"`
	Capability      VerificationDimension   `json:"capability"`
	Evidence        []VerificationEvidence  `json:"evidence"`
	Probes          []VerificationProbe     `json:"probes"`
	ProbePlan       []VerificationProbe     `json:"probe_plan,omitempty"`
	CurrentProbe    *VerificationProbe      `json:"current_probe,omitempty"`
	Passed          int                     `json:"passed"`
	Total           int                     `json:"total"`
}
