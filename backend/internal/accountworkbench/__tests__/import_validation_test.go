package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestInvalidAccountStopsBeforeManagementImportAndReportsSpecificReason(t *testing.T) {
	for _, tc := range []struct{ name, field, value, reason string }{
		{"malformed token reports format error", "access_token", "invalid-token", "令牌格式"},
		{"workspace mismatch reports identity error", "chatgpt_account_id", "different-workspace", "工作区不一致"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, private := fixture(t, `{}`)
			owner, _ := previewOwner(t, private)
			runner, _ := runTasks(t, service)
			var input map[string]any
			if err := json.Unmarshal([]byte(signedRunInput(t, service)), &input); err != nil {
				t.Fatal(err)
			}
			input[tc.field] = tc.value
			raw, _ := json.Marshal(input)
			requests := 0
			service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("invalid account must not reach management")
			}))
			ctx := context.Background()
			preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "import", Content: string(raw)})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.Start(ctx, owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			run, err = service.Run(ctx, owner, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 0 || run.Items[0].Status != "failed" || run.Items[0].AccountID != "" || !strings.Contains(run.Items[0].Message, tc.reason) {
				t.Fatalf("invalid account did not stop with specific reason: requests=%d message=%s", requests, run.Items[0].Message)
			}
		})
	}
}
