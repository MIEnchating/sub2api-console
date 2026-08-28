import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ManualAuthForm,
  ManualAuthHeadersEditor,
  manualAuthCompletion,
  manualAuthIncomplete,
} from "../../App";

function renderForm(upstreamType: string) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <ManualAuthForm host="api.example.test" upstreamType={upstreamType} />
    </QueryClientProvider>,
  );
}

describe("manual upstream authentication form", () => {
  it("shows Token fields for Sub2API and disables empty submission", () => {
    const markup = renderForm("sub2api");

    expect(markup).toContain(">Token<");
    expect(markup).toContain(">刷新 Token<");
    expect(markup).not.toContain(">Admin Key<");
    expect(markup).not.toContain(">User ID<");
    expect(markup).toMatch(/type="submit"[^>]*disabled=""/);
  });

  it("shows Admin Key and User ID fields for New API", () => {
    const markup = renderForm("newapi");

    expect(markup).toContain(">Admin Key<");
    expect(markup).toContain(">User ID<");
    expect(markup).not.toContain(">Token<");
    expect(markup).not.toContain(">刷新 Token<");
    expect(markup).toContain("自定义 Headers");
    expect(markup).not.toContain("Headers JSON");
  });

  it("keeps credential fields inside the dialog width", () => {
    const markup = renderForm("sub2api");

    expect(markup).toContain("min-w-0 max-w-full");
    expect(markup).toContain("overflow-x-clip");
    expect(markup).toContain("sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]");
  });

  it("uses the dialog as the only scroll container for long headers", () => {
    const markup = renderToStaticMarkup(
      <ManualAuthHeadersEditor value="long-token" onChange={() => undefined} />,
    );

    expect(markup).toContain('wrap="soft"');
    expect(markup).toContain("max-h-none");
    expect(markup).toContain("overflow-hidden");
    expect(markup).toContain("resize-none");
    expect(markup).toContain("[overflow-wrap:anywhere]");
    expect(markup).not.toContain("max-h-96");
    expect(markup).not.toContain("overflow-y-auto");
  });

  it("allows omitted mode credentials only when custom headers are configured", () => {
    const empty = { accessToken: "", refreshToken: "", adminKey: "", userId: "" };

    expect(manualAuthIncomplete("sub2api_user_token", empty, true)).toBe(false);
    expect(manualAuthIncomplete("newapi_admin_key", empty, true)).toBe(false);
    expect(manualAuthIncomplete("newapi_user_token", empty, true)).toBe(false);
    expect(manualAuthIncomplete("bearer_token", empty, true)).toBe(false);
    expect(manualAuthIncomplete("custom_headers", empty, true)).toBe(false);
    expect(manualAuthIncomplete("sub2api_user_token", empty, false)).toBe(true);
    expect(
      manualAuthIncomplete("newapi_admin_key", { ...empty, adminKey: "partial-admin-key" }, true),
    ).toBe(true);
  });

  it("replaces the old failed task state after credentials are verified", () => {
    const completion = manualAuthCompletion("api.example.test", {
      host: "api.example.test",
      verified: true,
      balance_sync: {
        status: "failed",
        balance_status: "读取失败",
        reason: "余额接口不可用",
      },
    });

    expect(completion.actionDialog).toBeNull();
    expect(completion.actionTaskId).toBeNull();
    expect(completion.notice).toBe(
      "api.example.test 凭证已验证并保存，余额同步失败：余额接口不可用",
    );
  });
});
