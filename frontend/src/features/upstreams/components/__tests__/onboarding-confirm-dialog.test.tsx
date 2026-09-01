import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Dialog } from "@/components/ui/dialog";
import { OnboardingConfirmContent } from "../onboarding-confirm-dialog";

describe("OnboardingConfirmContent", () => {
  it("previews every binding field before account creation", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <OnboardingConfirmContent
          items={[
            {
              upstream: "示例上游",
              upstreamGroup: "codex-special",
              multiplier: "0.15",
              localGroup: "codex",
              concurrency: 100,
              priority: 10,
              status: "待更新",
            },
          ]}
          pending={false}
          onCancel={() => undefined}
          onConfirm={() => undefined}
        />
      </Dialog>,
    );

    expect(markup).toContain("确认账号绑定变更");
    expect(markup).toContain("示例上游");
    expect(markup).toContain("codex-special");
    expect(markup).toContain("账号成本");
    expect(markup).toContain("0.15");
    expect(markup).not.toContain("本地分组售价");
    expect(markup).toContain("codex");
    expect(markup).toContain("100");
    expect(markup).toContain("待更新");
    expect(markup).toContain("确认提交 1 项变更");
    expect(markup).toContain('data-table-panel=""');
  });
});
