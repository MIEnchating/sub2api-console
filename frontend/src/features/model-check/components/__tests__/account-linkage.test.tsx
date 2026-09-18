import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type ModelCheckAccountStatus, type Task } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import { RegularCheckPanel } from "../regular-check-panel";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(id = "41") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  const accounts = [
    { ...account, id: "41", name: "同名账号" },
    { ...account, id: "42", name: "同名账号" },
  ];
  client.setQueryData(["accounts"], accounts);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-5.6-sol"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", id], { models: ["gpt-5.6-sol"] });
  vi.spyOn(api, "accounts").mockResolvedValue(accounts);
  vi.spyOn(api, "modelCheckAccountStatuses").mockResolvedValue([]);
  const back = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel accountID={id} onBackToAccounts={back} />
    </QueryClientProvider>,
  );
  return { client, back };
}

it("账号管理跳转后按 ID 预选同名账号，选择模型后提交正确账号", async () => {
  const task: Task = {
    id: "model-1",
    skill: "sub2api-model-check",
    operation: "test",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-09-18T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
    result: { tests: [] },
  };
  const run = vi.spyOn(api, "runModelCheck").mockResolvedValue(task);
  vi.spyOn(api, "task").mockResolvedValue(task);
  const { back } = setup();
  const table = screen.getByTestId("model-check-account-desktop-table");
  const boxes = within(table).getAllByRole("checkbox");
  expect(boxes[0]).toBeChecked();
  expect(boxes).toHaveLength(1);
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  const status: ModelCheckAccountStatus = {
    account_id: "41",
    status: "consistent",
    checked_at: task.updated_at,
    task_id: task.id,
    confidence: {
      evaluated_at: task.updated_at,
      short: { score: 20.7, samples: 1, passed: 1, failed: 0, inconclusive: 0 },
      long: { score: 20.7, samples: 1, passed: 1, failed: 0, inconclusive: 0 },
    },
  };
  vi.mocked(api.modelCheckAccountStatuses).mockResolvedValue([status]);
  fireEvent.click(screen.getByRole("checkbox", { name: /gpt-5.6-sol/ }));
  fireEvent.click(screen.getByRole("button", { name: /开始检测/ }));
  await waitFor(() =>
    expect(run).toHaveBeenCalledWith(
      { account_ids: ["41"], models: ["gpt-5.6-sol"], rounds: 1, timeout_seconds: 45 },
      expect.anything(),
    ),
  );
  await waitFor(() =>
    expect(within(table).getByRole("button", { name: "查看账号 41 检测统计" })).toHaveTextContent(
      "符合特征",
    ),
  );
  const matrixHeader = screen.getByText("检测矩阵").closest('[data-slot="card-header"]');
  for (const name of ["查看检测结果", "查看上次检测结果"]) {
    const button = screen.getByRole("button", { name });
    expect(matrixHeader).toContainElement(button);
    expect(button).toHaveClass("size-8");
  }
  fireEvent.click(screen.getByRole("button", { name: "查看检测结果" }));
  expect(await screen.findByRole("dialog", { name: "模型检测结果" })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "账号管理" }));
  expect(back).toHaveBeenCalledOnce();
});

it("链接账号已删除时清除选择并禁止提交", async () => {
  setup("99");
  await waitFor(() => expect(screen.queryByText("同名账号（#99）")).not.toBeInTheDocument());
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  expect(screen.queryByRole("checkbox", { name: /gpt-5.6-sol/ })).not.toBeInTheDocument();
});

it("上次结果尚未读回时展示加载状态，读取失败保留关闭和重试入口", async () => {
  let rejectTask!: (error: Error) => void;
  vi.spyOn(api, "task").mockImplementation(
    () =>
      new Promise<Task>((_resolve, reject) => {
        rejectTask = reject;
      }),
  );
  const { client } = setup();
  await act(async () =>
    client.setQueryData(
      ["model-check-account-statuses"],
      [
        {
          account_id: "41",
          status: "consistent",
          checked_at: "2026-09-18T00:00:00Z",
          task_id: "previous",
        },
      ],
    ),
  );
  fireEvent.click(await screen.findByRole("button", { name: "查看上次检测结果" }));
  expect(await screen.findByRole("status", { name: "正在读取检测结果" })).toBeVisible();
  await act(async () => rejectTask(new Error("历史记录暂时不可用")));
  const dialog = screen.getByRole("dialog");
  expect(await within(dialog).findByRole("button", { name: "重新读取" })).toBeEnabled();
  fireEvent.click(within(dialog).getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});
