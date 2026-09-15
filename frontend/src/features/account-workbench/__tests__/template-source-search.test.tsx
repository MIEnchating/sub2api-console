import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchTemplateSourcePicker } from "../components/workbench-template-source";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("按分组搜索线上模板来源时仅列出匹配账号，清空搜索恢复列表", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "团队账号", platform: "openai", account_type: "oauth", groups: ["共享组"] },
      { id: "42", name: "个人账号", platform: "openai", account_type: "oauth", groups: ["个人组"] },
    ],
  );
  render(
    <QueryClientProvider client={client}>
      <WorkbenchTemplateSourcePicker
        disabled={false}
        onApply={() => undefined}
        onBlockedChange={() => undefined}
      />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "搜索线上账号" }), "共享组");
  await user.click(screen.getByRole("combobox", { name: "来源账号" }));
  expect(screen.getByRole("option", { name: "团队账号（ID 41）" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "个人账号（ID 42）" })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.clear(screen.getByRole("textbox", { name: "搜索线上账号" }));
  screen.getByRole("combobox", { name: "来源账号" }).focus();
  await user.keyboard("{ArrowDown}");
  expect(await screen.findAllByRole("option")).toHaveLength(2);
});
