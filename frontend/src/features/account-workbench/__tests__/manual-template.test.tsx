import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ManualTemplateEditor } from "../components/manual-template-editor";
import {
  manualTemplateConfig,
  manualTemplateDefaults,
  manualTemplateSchema,
} from "../lib/manual-template-schema";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(fetcher: typeof fetch): void {
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ManualTemplateEditor revision={3} onClose={() => undefined} />
    </QueryClientProvider>,
  );
}
it("无需线上账号即可保存手动模板，并保留零并发和高精度倍率", async () => {
  let payload: Record<string, unknown> | undefined;
  const fetcher = vi.fn(async (_url, init) => {
    payload = JSON.parse(String(init?.body));
    return Response.json({ revision: 4, preferred_id: "manual", items: [] });
  });
  mount(fetcher);
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "模板名称" }), "手动配置");
  await user.clear(screen.getByRole("spinbutton", { name: "并发数" }));
  await user.type(screen.getByRole("spinbutton", { name: "并发数" }), "0");
  await user.clear(screen.getByRole("textbox", { name: "计费倍率" }));
  await user.type(screen.getByRole("textbox", { name: "计费倍率" }), "0.1234567890123456789");
  await user.type(screen.getByRole("textbox", { name: "分组 ID" }), "7, 8");
  expect(fetcher).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "保存模板" }));
  await waitFor(() =>
    expect(payload).toMatchObject({
      name: "手动配置",
      revision: 3,
      config: { concurrency: 0, rate_multiplier: "0.1234567890123456789", group_ids: ["7", "8"] },
    }),
  );
  expect(payload).not.toHaveProperty("source_id");
  expect(payload).not.toHaveProperty("source_version");
});
it("无效名称和分组阻止保存并提示具体字段", async () => {
  const fetcher = vi.fn();
  mount(fetcher);
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "分组 ID" }), "abc");
  await user.click(screen.getByRole("button", { name: "保存模板" }));
  expect(await screen.findByText("请输入模板名称")).toBeVisible();
  expect(screen.getByText("请输入分组 ID，多个 ID 用逗号分隔")).toBeVisible();
  expect(fetcher).not.toHaveBeenCalled();
});
it("更新手动模板保留未编辑的附加开关和到期时间", () => {
  const original = {
    ...manualTemplateConfig({ ...manualTemplateDefaults(), name: "配置" }),
    expires_at: 1900000000,
    extra: { openai_ws_enabled: true, codex_fingerprint_mode: "full" },
  };
  const result = manualTemplateConfig(
    { ...manualTemplateDefaults(), name: "更新", fingerprint: "session" },
    original,
  );
  expect(result.expires_at).toBe(1900000000);
  expect(result.extra).toEqual({ openai_ws_enabled: true, codex_fingerprint_mode: "session" });
});
it.each(["[]", '{"gpt-5":12}', "not-json"])(
  "模型映射 %s 不是字符串映射时禁止提交",
  (model_mapping) => {
    expect(
      manualTemplateSchema.safeParse({ ...manualTemplateDefaults(), name: "配置", model_mapping })
        .success,
    ).toBe(false);
  },
);
