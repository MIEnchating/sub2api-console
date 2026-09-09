import type { QueryClient } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "@/api";

import { renderOnboarding, upstream } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("新上游验证成功后自动获取开户所需的上游信息", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(api, "detectUpstream").mockResolvedValue({
    base_url: upstream.base_url,
    host: upstream.host,
    upstream_type: null,
    auth_mode: null,
    name: null,
    type_detected: false,
    name_detected: false,
    evidence: null,
  });
  vi.spyOn(api, "createUpstream").mockResolvedValue(upstream);
  client = renderOnboarding();

  await user.type(await screen.findByLabelText("上游地址", { exact: true }), upstream.host);
  await user.click(screen.getByRole("combobox", { name: "鉴权方式" }));
  await user.click(await screen.findByRole("option", { name: "Token + 刷新 Token" }));
  await user.type(screen.getByLabelText("Token", { exact: true }), "access-token");
  await user.type(screen.getByLabelText("刷新 Token", { exact: true }), "refresh-token");
  await user.click(screen.getByRole("button", { name: "添加并验证上游" }));

  await waitFor(() =>
    expect(api.prepareOnboarding).toHaveBeenCalledWith(upstream.host, expect.any(Object)),
  );
  expect(await screen.findByRole("region", { name: "当前上游概况" })).toBeVisible();
});
