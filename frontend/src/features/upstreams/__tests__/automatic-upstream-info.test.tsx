import type { QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
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

it.each([false, true])("新上游手动命名=%s时保存正确名称来源并自动获取开户信息", async (manual) => {
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
  vi.mocked(api.upstreamOptions).mockResolvedValue([]);
  await act(async () => {
    await client!.invalidateQueries({ queryKey: ["upstreams", "options"] });
  });

  await user.click(await screen.findByLabelText("上游地址", { exact: true }));
  await user.paste(upstream.host);
  await user.click(screen.getByRole("combobox", { name: "鉴权方式" }));
  await user.click(await screen.findByRole("option", { name: "Token + 刷新 Token" }));
  await user.click(screen.getByLabelText("Token", { exact: true }));
  await user.paste("access-token");
  await user.click(screen.getByLabelText("刷新 Token", { exact: true }));
  await user.paste("refresh-token");
  if (manual) {
    await user.click(screen.getByLabelText("名称", { exact: true }));
    await user.paste("自定义站点");
  }
  await user.click(screen.getByRole("button", { name: "添加并验证上游" }));

  await waitFor(() =>
    expect(api.prepareOnboarding).toHaveBeenCalledWith(upstream.host, expect.any(Object)),
  );
  expect(await screen.findByRole("region", { name: "当前上游概况" })).toBeVisible();
  expect(api.createUpstream).toHaveBeenCalledWith(
    expect.objectContaining({
      name_mode: manual ? "manual" : "auto",
      name: manual ? "自定义站点" : upstream.host,
    }),
    expect.anything(),
  );
});
