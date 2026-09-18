import type { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { boundCandidate, renderOnboarding } from "../../__tests__/onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});

it("批量底栏移除备注并保留并发、优先级和预览入口", async () => {
  client = renderOnboarding(boundCandidate("active"), false);
  const bar = await screen.findByRole("toolbar", { name: "批量添加账号" });

  expect(bar).toHaveClass("flex-wrap", "shrink-0");
  expect(within(bar).queryByRole("textbox", { name: /备注/ })).not.toBeInTheDocument();
  expect(within(bar).getByRole("spinbutton", { name: "并发" })).toHaveValue(null);
  expect(within(bar).getByRole("spinbutton", { name: "优先级" })).toHaveValue(1);
  expect(bar.querySelectorAll('[data-slot="field-error"]')).toHaveLength(0);
  expect(within(bar).getByRole("button", { name: "预览 0 项变更" })).toBeDisabled();
});
