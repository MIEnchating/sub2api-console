import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import type { AnimationSchedule } from "@/api";
import { AnimationCheckPanel } from "../animation-check-panel";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  toast.dismiss();
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("批量设置遇到版本冲突时保留草稿和原计划并显示失败原因", async () => {
  const view = setup();
  const close = vi.fn();
  view.client.setQueryData(["model-animation", "schedules"], [base]);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({ detail: "账号 41 的自动检测配置已变化，请刷新后重试" }, { status: 422 }),
    ),
  );
  render(
    <QueryClientProvider client={view.client}>
      <Toaster />
      <AnimationScheduleDialog
        accountID="41"
        accountName="2 个账号"
        model="batch-model"
        targets={[
          { accountID: "41", accountName: "已有计划", schedule: base },
          { accountID: "42", accountName: "新计划" },
        ]}
        onClose={close}
      />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await user.click(screen.getByRole("button", { name: "确认保存并开启" }));
  expect(await screen.findByText(/账号 41 的自动检测配置已变化/)).toBeVisible();
  expect(screen.queryByRole("dialog", { name: "确认开启自动检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("batch-model");
  expect(screen.getByRole("button", { name: "保存设置" })).toBeEnabled();
  expect(view.client.getQueryData(["model-animation", "schedules"])).toEqual([base]);
  expect(close).not.toHaveBeenCalled();
});
const base: AnimationSchedule = {
  account_id: "41",
  enabled: true,
  model: "saved-model",
  interval_minutes: 30,
  timeout_seconds: 120,
  version: 2,
};
function setup() {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  const saves: { schedules: AnimationSchedule[] }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") saves.push(JSON.parse(String(init.body)));
      return Response.json([]);
    }),
  );
  return { client, saves };
}

it("在前置检测 Tab 批量设置每日时间时只保存前置计划并保留各账号版本", async () => {
  const view = setup();
  view.client.setQueryData(
    ["accounts"],
    ["41", "42"].map((id) => ({ id, name: id, groups: [], platform: "openai" })),
  );
  view.client.setQueryData(["model-animation", "history"], []);
  view.client.setQueryData(
    ["model-animation", "schedules"],
    [base, { ...base, mode: "precheck", version: 5 }],
  );
  render(
    <QueryClientProvider client={view.client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("button", { name: "批量自动检测设置" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  await user.click(screen.getByRole("button", { name: "批量自动检测设置" }));
  const dialog = screen.getByRole("dialog", { name: /批量自动前置检测设置/ });
  expect(within(dialog).queryByRole("group", { name: "自动检测内容" })).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole("checkbox", { name: "开启自动检测" }));
  await user.type(within(dialog).getByRole("textbox", { name: "检测模型" }), "daily-model");
  const daily = within(dialog).getByRole("radio", { name: "每天定时" });
  daily.focus();
  await user.keyboard(" ");
  expect(daily).toBeChecked();
  fireEvent.change(within(dialog).getByLabelText("每天检测时间（北京时间）"), {
    target: { value: "09:35" },
  });
  await user.click(within(dialog).getByRole("button", { name: "保存设置" }));
  const confirmation = screen.getByRole("dialog", { name: "确认开启自动检测" });
  expect(confirmation).toHaveTextContent("每天 09:35（北京时间）");
  expect(confirmation).toHaveTextContent("ID 41");
  expect(confirmation).toHaveTextContent("ID 42");
  expect(view.saves).toHaveLength(0);
  await user.click(within(confirmation).getByRole("button", { name: "确认保存并开启" }));
  await waitFor(() => expect(view.saves).toHaveLength(1));
  expect(view.saves[0].schedules).toEqual([
    expect.objectContaining({
      account_id: "41",
      mode: "precheck",
      version: 5,
      schedule_type: "daily",
      daily_times: ["09:35"],
      timezone: "Asia/Shanghai",
    }),
    expect.objectContaining({
      account_id: "42",
      mode: "precheck",
      version: 0,
      precheck_questions: ["candy"],
    }),
  ]);
});

it("切换 Tab 后自动设置分别回显对应计划", async () => {
  const view = setup();
  view.client.setQueryData(
    ["accounts"],
    [{ id: "41", name: "测试", groups: [], platform: "openai" }],
  );
  view.client.setQueryData(["model-animation", "history"], []);
  view.client.setQueryData(
    ["model-animation", "schedules"],
    [base, { ...base, mode: "precheck", model: "precheck-model", version: 5 }],
  );
  render(
    <QueryClientProvider client={view.client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "自动检测设置" }));
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("saved-model");
  expect(screen.queryByRole("button", { name: "选择前置检测题目" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  await user.click(screen.getByRole("button", { name: "自动检测设置" }));
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("precheck-model");
  expect(screen.getAllByRole("button", { name: "选择前置检测题目" }).length).toBeGreaterThan(0);
});

it("选择每天定时但未填时间时阻止保存并允许取消", async () => {
  const view = setup();
  const close = vi.fn();
  render(
    <QueryClientProvider client={view.client}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试"
        model="test-model"
        onClose={close}
      />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
  await user.click(screen.getByRole("radio", { name: "每天定时" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("请选择每天检测的时间")).toBeVisible();
  expect(screen.getByLabelText("每天检测时间（北京时间）")).toHaveAttribute("aria-invalid", "true");
  expect(view.saves).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(close).toHaveBeenCalledOnce();
});
