import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchMixedForm } from "../components/workbench-mixed-form";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function mount(): ReturnType<typeof vi.fn> {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const submit = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMixedForm onSubmit={submit} />
    </QueryClientProvider>,
  );
  return submit;
}

it("默认导入提供导入并检测主操作，空内容不能启动", async () => {
  mount();
  expect(await screen.findByRole("button", { name: "导入并检测" })).toBeDisabled();
  await userEvent.setup().type(screen.getByRole("textbox", { name: "账号内容" }), "rt_reference");
  expect(screen.getByRole("button", { name: "导入并检测" })).toBeEnabled();
});

it("默认导入预览包含原版检测模型且高级设置没有额外检测开关", async () => {
  const submit = mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_reference");
  await user.click(screen.getByRole("button", { name: "高级设置" }));
  expect(screen.queryByRole("checkbox", { name: "导入后执行模型检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("gpt-5.6-sol");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ check_after_import: true, model: "gpt-5.6-sol" }),
    ),
  );
});

it("切换仅导出JSON后主操作变为生成JSON且不会发送检测或模板配置", async () => {
  const submit = mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_reference");
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  await user.click(screen.getByRole("button", { name: "生成 JSON" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        scope: "local-export",
        check_after_import: false,
        model: "",
        template_id: undefined,
      }),
    ),
  );
});
