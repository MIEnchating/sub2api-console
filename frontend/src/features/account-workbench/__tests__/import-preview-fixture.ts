import type { WorkbenchPreview } from "../types";

export function importPreviewFixture(count: number): WorkbenchPreview {
  return {
    id: "preview-one",
    revision: 1,
    expires_at: "2099-01-01T00:00:00Z",
    action: "import",
    check: true,
    promote: true,
    template: null,
    items: Array.from({ length: count }, (_, index) => ({
      id: `account-${index + 1}`,
      index,
      kind: "refresh_token",
      name: "",
      email: `account-${index + 1}@example.test`,
      plan: "",
      identity_source: "pending_refresh",
    })),
    errors: [],
    duplicate_count: 0,
  };
}
