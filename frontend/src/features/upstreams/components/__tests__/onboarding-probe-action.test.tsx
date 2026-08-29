import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { OnboardingProbeAction } from "../onboarding-probe-action";

describe("OnboardingProbeAction", () => {
  it("enables probing before a local account is added", () => {
    const markup = renderToStaticMarkup(
      <OnboardingProbeAction
        target={{ host: "api.example", groupId: "6", name: "Codex" }}
        groupName="Codex"
        pending={false}
        onProbe={() => undefined}
      />,
    );

    expect(markup).toContain("探活");
    expect(markup).toContain("探活测试");
    expect(markup).not.toContain("探活测试 Codex");
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).not.toContain("title=");
    expect(markup.match(/<button[^>]*>/)?.[0]).not.toMatch(/\sdisabled(?:=|\s|>)/);
  });

  it("disables probing only when the upstream group has no stable target", () => {
    const markup = renderToStaticMarkup(
      <OnboardingProbeAction
        target={null}
        groupName="Codex"
        pending={false}
        onProbe={() => undefined}
      />,
    );

    expect(markup).toContain("当前不可探活");
    expect(markup).not.toContain("Codex 当前不可探活");
    expect(markup).toContain("disabled");
  });
});
