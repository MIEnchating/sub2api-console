import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { WorkbenchHistory } from "../components/workbench-history";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const task: Task = {
  id: "batch-one",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "succeeded",
  progress: 100,
  message: "本批处理完成",
  result: { email: "alice@example.test" },
  created_at: "2026-09-15T00:00:00Z",
  updated_at: "2026-09-15T00:00:00Z",
};

function mount(tasks: Task[] = [task], active = false): ReturnType<typeof vi.fn> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url) =>
      Response.json(String(url).endsWith("/history") ? tasks : task),
    ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const resume = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <WorkbenchHistory activeTaskId={active ? task.id : null} onContinue={resume} />
    </QueryClientProvider>,
  );
  return resume;
}

it("打开记录只展示批次结果，不再提供通用再生、重新授权或批量管理入口", async () => {
  mount();
  await userEvent.setup().click(await screen.findByRole("button", { name: "查看任务 batch-one" }));
  expect(screen.getByRole("dialog", { name: "处理详情" })).toBeVisible();
  expect(screen.getByRole("region", { name: "本批处理详情" })).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /重新生成授权文件|从此记录重新授权|取消全部|删除选中/ }),
  ).not.toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "筛选处理类型" })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "关闭详情" }));
  expect(screen.queryByRole("region", { name: "本批处理详情" })).not.toBeInTheDocument();
});

it("有当前批次时可从记录返回继续处理", async () => {
  const resume = mount([task], true);
  await userEvent.setup().click(await screen.findByRole("button", { name: "继续当前批次" }));
  expect(resume).toHaveBeenCalledOnce();
});

it("按邮箱搜索后只显示匹配记录，清空后恢复列表", async () => {
  mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "搜索处理记录" }), "missing");
  expect(screen.queryByRole("button", { name: "查看任务 batch-one" })).not.toBeInTheDocument();
  await user.clear(screen.getByRole("textbox", { name: "搜索处理记录" }));
  await user.type(screen.getByRole("textbox", { name: "搜索处理记录" }), "ALICE@example.test");
  expect(screen.getByRole("button", { name: "查看任务 batch-one" })).toBeVisible();
});

it("没有处理记录时显示空态且不会出现无效批量操作", async () => {
  mount([]);
  await waitFor(() => expect(screen.getByText("暂无处理记录")).toBeVisible());
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});

it("历史授权任务等待操作时可接管原浏览器，关闭详情不会取消或重新创建授权", async () => {
  const oauthTask: Task = {
    ...task,
    id: "oauth-existing",
    operation: "account-workbench-oauth",
    status: "waiting_input",
  };
  mount([oauthTask]);
  const fetcher = vi.fn<typeof fetch>(async (url) => {
    if (String(url).endsWith("/history")) return Response.json([oauthTask]);
    if (String(url).includes("/oauth/"))
      return Response.json({
        id: oauthTask.id,
        task_id: oauthTask.id,
        status: "waiting",
        host: "auth.openai.com",
        message: "请完成原账号验证",
        expires_at: "2099-01-01T00:00:00Z",
      });
    return Response.json(oauthTask);
  });
  vi.stubGlobal("fetch", fetcher);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "查看任务 oauth-existing" }));
  await user.click(screen.getByRole("button", { name: "继续授权" }));
  expect(await screen.findByRole("button", { name: "登录完成，验证授权" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "关闭详情" }));
  expect(
    fetcher.mock.calls.some(([, init]) => init?.method === "POST" || init?.method === "DELETE"),
  ).toBe(false);
  expect(
    fetcher.mock.calls.some(
      ([url]) => String(url) === "/api/account-workbench/oauth/oauth-existing",
    ),
  ).toBe(true);
});
