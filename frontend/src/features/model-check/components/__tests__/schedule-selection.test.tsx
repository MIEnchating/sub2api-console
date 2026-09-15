import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AnimationSchedule } from "@/api";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(schedule?: AnimationSchedule): void {
  const client = new QueryClient();
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试账号"
        model="gpt-6-astra"
        schedule={schedule}
        onClose={() => {}}
      />
    </QueryClientProvider>,
  );
}

function captureSaves(): unknown[] {
  const saves: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      saves.push(JSON.parse(String(init?.body)));
      return Response.json([]);
    }),
  );
  return saves;
}

it("同时勾选两项后确认执行顺序并保存组合模式", async () => {
  const saves = captureSaves();
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
  const precheck = screen.getByRole("checkbox", { name: "前置检测" });
  precheck.focus();
  await user.keyboard(" ");
  expect(precheck).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "动画检测" })).toBeChecked();
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  const confirm = screen.getByRole("dialog", { name: "确认开启自动检测" });
  expect(confirm).toHaveTextContent("随后生成一次动画");
  expect(saves).toHaveLength(0);
  await user.click(within(confirm).getByRole("button", { name: "确认保存并开启" }));
  await waitFor(() =>
    expect(saves).toEqual([
      expect.objectContaining({ mode: "both", precheck_questions: ["candy", "knowledge-cutoff"] }),
    ]),
  );
  expect(saves[0]).not.toHaveProperty("detection_types");
});

it("取消全部检测内容时提示校验错误且不能进入保存确认", async () => {
  const saves = captureSaves();
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "动画检测" }));
  await user.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("请选择至少一种检测内容");
  expect(screen.queryByRole("dialog", { name: "确认开启自动检测" })).not.toBeInTheDocument();
  expect(saves).toHaveLength(0);
});

it("重新打开组合计划后可取消前置检测并只保存动画检测", async () => {
  const saves = captureSaves();
  mount({
    account_id: "41",
    model: "gpt-6-astra",
    mode: "both",
    precheck_questions: ["candy"],
    enabled: true,
    interval_minutes: 10,
    timeout_seconds: 120,
    version: 2,
  });
  const user = userEvent.setup();
  expect(screen.getByRole("checkbox", { name: "动画检测" })).toBeChecked();
  await user.click(screen.getByRole("checkbox", { name: "前置检测" }));
  expect(screen.queryByRole("button", { name: "选择前置检测题目" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await user.click(
    within(screen.getByRole("dialog", { name: "确认开启自动检测" })).getByRole("button", {
      name: "确认保存并开启",
    }),
  );
  await waitFor(() => expect(saves).toEqual([expect.objectContaining({ version: 2 })]));
  expect(saves[0]).not.toHaveProperty("mode");
  expect(saves[0]).not.toHaveProperty("precheck_questions");
});
