import { describe, expect, it } from "vitest";

import type { UpstreamConfiguration } from "@/api";
import { upstreamEditDialogLayout, upstreamEditPresentation } from "../upstream-edit-dialog";

describe("upstream edit dialog", () => {
  it("shows redacted credential state and balance mapping summary without group details", () => {
    const configuration: UpstreamConfiguration = {
      host: "api.example.test",
      name: "Example",
      base_url: "https://api.example.test",
      upstream_type: "newapi",
      auth_mode: "newapi_admin_key",
      recharge_rate: "5",
      raw_balance: "25",
      balance: "5",
      has_access_token: false,
      has_refresh_token: false,
      has_admin_key: true,
      has_user_id: true,
      headers: {},
      header_names: [],
      cookie_names: [],
      groups: [
        {
          host: "api.example.test",
          group_id: "codex",
          name: "codex",
          description: "Codex pool",
          platform: "openai",
          status: "active",
          raw_rate: "0.2",
          effective_rate: "0.04",
          recharge_rate: "5",
          bound: false,
          bound_accounts: [],
          key_present: true,
          bindable: true,
          unavailable_reason: null,
        },
      ],
    };
    const presentation = upstreamEditPresentation(configuration);

    expect(presentation.adminKeyState).toBe("已配置，留空则不修改");
    expect(presentation.rawBalance).toBe("25");
    expect(presentation.mappedBalance).toBe("5");
    expect(presentation).not.toHaveProperty("groups");
    expect(JSON.stringify(presentation)).not.toContain("secret");
  });

  it("keeps vertical scrolling without exposing a horizontal scroll area", () => {
    expect(upstreamEditDialogLayout.content).toContain("min-w-0");
    expect(upstreamEditDialogLayout.content).toContain("overflow-hidden");
    expect(upstreamEditDialogLayout.scrollArea).toContain("min-w-0");
    expect(upstreamEditDialogLayout.scrollArea).toContain("overflow-x-clip");
    expect(upstreamEditDialogLayout.scrollArea).toContain("overflow-y-auto");
    expect(upstreamEditDialogLayout.form).toContain("min-w-0");
    expect(upstreamEditDialogLayout.form).toContain("max-w-full");
  });
});
