import type { QueryClient } from "@tanstack/react-query";
import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["active", "disabled"])("直达 %s 的已有绑定分组时允许预览本地分组变更", async (status) => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = renderOnboarding(boundCandidate(status));
  fireEvent.click(await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "已有绑定分组 本地分组" }), {
    key: "Escape",
  });
  fireEvent.click(screen.getByRole("button", { name: "预览更新绑定" }));
  const confirmation = await screen.findByRole("dialog", { name: "确认账号绑定变更" });
  expect(within(confirmation).getByRole("cell", { name: "原分组、备用分组" })).toBeVisible();
  expect(within(confirmation).getByRole("cell", { name: "待更新" })).toBeVisible();
});
