import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { task } from "../../components/__tests__/fixtures";
import { useTaskCompletion } from "../use-task-completion";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each([
  { name: "取消刚持久化", result: {}, message: /不会自动撤销.*核对远端状态/ },
  {
    name: "部分资源已确认",
    result: { saved: true, resource_id: 37 },
    message: /不会自动撤销.*核对远端状态/,
  },
  { name: "排队期间取消", result: { remote_write: false }, message: /未执行远端操作/ },
])("$name 时返回明确取消原因且不把取消当作成功", async (scenario) => {
  const fetch = vi.fn(async () =>
    Response.json({
      ...task,
      status: "cancelled",
      message: "任务已手动取消",
      result: scenario.result,
    }),
  );
  vi.stubGlobal("fetch", fetch);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = renderHook(useTaskCompletion, {
    wrapper: (props) => <QueryClientProvider client={client}>{props.children}</QueryClientProvider>,
  });
  let completion!: Promise<unknown>;
  act(() => {
    completion = view.result.current
      .wait({ ...task, status: "running" })
      .catch((error: unknown) => error);
  });
  let error: unknown;
  await act(async () => {
    error = await completion;
  });
  expect(error).toMatchObject({
    code: "kuma_task_cancelled",
    message: expect.stringMatching(scenario.message),
  });
  expect(view.result.current.task).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});
