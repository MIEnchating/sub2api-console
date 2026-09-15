package accountworkbench

type LoginProfileView struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	Revision    int64  `json:"revision"`
	UpdatedAt   string `json:"updated_at"`
	HasPassword bool   `json:"has_password"`
	HasTOTP     bool   `json:"has_totp"`
	HasProxy    bool   `json:"has_proxy"`
	MailKind    string `json:"mail_kind,omitempty"`
	SMSProvider string `json:"sms_provider,omitempty"`
}

type LoginProfileSaveInput struct {
	ID        string          `json:"id,omitempty"`
	AccountID string          `json:"account_id"`
	Revision  int64           `json:"revision"`
	Confirmed bool            `json:"confirmed"`
	Login     OAuthLoginInput `json:"login"`
}

type ReauthorizationPreviewInput struct {
	SourceTaskID  string   `json:"source_task_id,omitempty"`
	AccountIDs    []string `json:"account_ids"`
	FailedBatchID string   `json:"failed_batch_id,omitempty"`
	FreshLogin    bool     `json:"fresh_login"`
}

type LoginProfileSecurityInput struct {
	ProfileID  string `json:"profile_id"`
	Revision   int64  `json:"revision"`
	SecurityID string `json:"security_id"`
	BatchID    string `json:"batch_id,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	Confirmed  bool   `json:"confirmed"`
}
