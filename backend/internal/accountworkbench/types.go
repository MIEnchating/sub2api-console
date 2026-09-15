package accountworkbench

import "github.com/MIEnchating/sub2api-console/backend/internal/configstore"

// InputItem is private execution input. Credentials never serialize into a
// preview, task result, audit record, or other browser-facing response.
type InputItem struct {
	Index       int            `json:"index"`
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Email       string         `json:"email"`
	PlanType    string         `json:"plan_type"`
	Credentials map[string]any `json:"-"`
}

type InputError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

type TemplateConfig = configstore.WorkbenchTemplateConfig
