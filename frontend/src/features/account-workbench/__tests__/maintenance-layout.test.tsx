import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchMaintenancePanel } from "../components/workbench-maintenance";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.maintenance, {
    enabled: false,
    interval_minutes: 5,
    cooldown_minutes: 10,
    group_ids: [],
    model: "",
    check_after_repair: false,
    revision: 1,
  });
  client.setQueryData(["groups"], []);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMaintenancePanel />
    </QueryClientProvider>,
  );
}

it("维护默认只显示常用参数，键盘展开后可编辑高级设置", async () => {
  mount();
  expect(screen.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "检测模型", hidden: true })).not.toBeVisible();
  const summary = screen.getByText("高级设置");
  summary.focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(screen.getByRole("textbox", { name: "检测模型" })).toBeDisabled();
  expect(screen.queryByRole("region", { name: "待上传授权" })).not.toBeInTheDocument();
});

it("开启修复后检测而未填写模型时自动展开字段，阻止无效保存", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "修复后检测（会产生模型调用用量）" }));
  await user.click(screen.getByRole("button", { name: "保存维护设置" }));
  expect(await screen.findByRole("textbox", { name: "检测模型" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveAttribute("aria-invalid", "true");
  expect(fetcher).not.toHaveBeenCalled();
});
