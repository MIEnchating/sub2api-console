import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { WorkbenchRunInput } from "@/api";
import { WorkbenchMixedForm } from "../components/workbench-mixed-form";

let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json([])),
  );
});
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(onSubmit: (input: WorkbenchRunInput) => void = () => {}): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMixedForm onSubmit={onSubmit} />
    </QueryClientProvider>,
  );
}

it("混合输入按实际容器宽度分栏且高级设置不提供短信供应商配置", async () => {
  mount();
  const input = await screen.findByRole("region", { name: "账号内容与文件" });
  const options = screen.getByRole("region", { name: "导入选项" });
  expect(options).not.toHaveAttribute("data-slot", "card");
  expect(input).not.toHaveAttribute("data-slot", "card");
  expect(screen.getByRole("form", { name: "账号导入输入" })).toHaveClass("@container/mixed");
  expect(screen.getByRole("group", { name: "混合内容与选项" })).toHaveClass(
    "grid",
    "min-w-0",
    "@3xl/mixed:grid-cols-[minmax(0,1fr)_20rem]",
  );
  expect(within(input).getByRole("textbox", { name: "账号内容" })).toHaveClass("h-64", "resize-y");
  const expand = within(options).getByRole("button", { name: "高级设置" });
  expect(expand).toHaveAttribute("aria-expanded", "false");
  expect(within(options).queryByRole("combobox", { name: "短信验证码" })).not.toBeInTheDocument();
  expand.focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(expand).toHaveAttribute("aria-expanded", "true");
  expect(within(options).getByLabelText("登录 / 检测代理")).toHaveAttribute("type", "password");
  expect(within(options).queryByRole("combobox", { name: "短信验证码" })).not.toBeInTheDocument();
  expect(
    within(options).queryByRole("checkbox", { name: "保存本批恢复资料" }),
  ).not.toBeInTheDocument();
});

it("收起登录设置后提交无效代理，会自动展开并显示可修正的字段", async () => {
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_fixture");
  const expand = screen.getByRole("button", { name: "高级设置" });
  await user.click(expand);
  await user.type(screen.getByLabelText("登录 / 检测代理"), "invalid-proxy");
  await user.click(expand);
  expect(expand).toHaveAttribute("aria-expanded", "false");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(expand).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByLabelText("登录 / 检测代理")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});

it("切换私有转换后忽略已隐藏的无效模型，不阻止预览或发送检测配置", async () => {
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "高级设置" }));
  await user.clear(screen.getByRole("textbox", { name: "检测模型" }));
  await user.click(screen.getByRole("textbox", { name: "检测模型" }));
  await user.paste("m".repeat(201));
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({
      scope: "local-export",
      export_only: true,
      template_id: undefined,
      check_after_import: false,
      model: "",
    }),
  );
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("账号内容为空时不留错误占位且禁用提交入口", async () => {
  mount();
  const input = await screen.findByRole("form", { name: "账号导入输入" });
  expect(input.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "导入并检测" })).toBeDisabled();
});

it("处理方式初始选择和切换后都显示中文标签而非协议值", async () => {
  mount();
  const user = userEvent.setup();
  const mode = await screen.findByRole("button", { name: "导入站点" });
  expect(mode).toHaveAttribute("aria-pressed", "true");
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  expect(mode).toHaveAttribute("aria-pressed", "false");
  expect(screen.getByRole("button", { name: "仅导出 JSON" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

it("清空账号内容后保留已选处理方式且不提交预览", async () => {
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  const content = await screen.findByRole("textbox", { name: "账号内容" });
  await user.type(content, "operator@example.test----fixture-password");
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  await user.click(screen.getByRole("button", { name: "清空输入" }));
  expect(content).toHaveValue("");
  expect(screen.getByRole("button", { name: "仅导出 JSON" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(submit).not.toHaveBeenCalled();
});

it("侧栏展开即显示默认检测模型，切换私有转换隐藏检测字段并保留账号原文", async () => {
  mount();
  const user = userEvent.setup();
  const content = await screen.findByRole("textbox", { name: "账号内容" });
  await user.type(content, "rt_layout_fixture");
  await user.click(screen.getByRole("button", { name: "高级设置" }));
  expect(screen.queryByRole("checkbox", { name: "导入后执行模型检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("gpt-5.6-sol");
  expect(screen.getByRole("textbox", { name: "检测模型" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  expect(content).toHaveValue("rt_layout_fixture");
});

it("异步读取文件时两栏与提交入口均禁用，读取完成后恢复并展示原文", async () => {
  let finishRead: (value: string) => void = () => undefined;
  const file = new File(["rt_file_fixture"], "accounts.txt", { type: "text/plain" });
  Object.defineProperty(file, "text", {
    value: () =>
      new Promise<string>((resolve) => {
        finishRead = resolve;
      }),
  });
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "高级设置" }));
  await user.upload(await screen.findByLabelText("从文件读取（最大 2 MB）"), file);
  await screen.findByText("正在读取文件");
  expect(screen.getByRole("textbox", { name: "账号内容" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "仅导出 JSON" })).toBeDisabled();
  expect(screen.getByLabelText("登录 / 检测代理")).toBeDisabled();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  await act(async () => {
    finishRead("rt_file_fixture");
  });
  expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveValue("rt_file_fixture");
  expect(screen.getByRole("button", { name: "仅导出 JSON" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
});

it("从统一输入预览账号时不发送短信配置且不启用恢复保存", async () => {
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  await user.type(
    await screen.findByRole("textbox", { name: "账号内容" }),
    "operator@example.test----fixture-password",
  );
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(submit).toHaveBeenCalledOnce();
  expect(submit.mock.calls[0][0]).not.toHaveProperty("sms");
  expect(submit.mock.calls[0][0]).toHaveProperty("recovery_enabled", false);
});

it("模板后台刷新失败后切换仅导出 JSON，仍可预览独立范围", async () => {
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_fixture");
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json({ detail: "模板读取失败" }, { status: 503 })),
  );
  await act(async () => {
    await client.refetchQueries();
  });
  await user.click(screen.getByRole("button", { name: "仅导出 JSON" }));
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({ scope: "local-export", export_only: true, template_id: undefined }),
  );
});

it("首次读取线上模板失败仍能选择仅导出 JSON", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json({ detail: "模板不可用" }, { status: 503 })),
  );
  const submit = vi.fn();
  mount(submit);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "仅导出 JSON" }));
  await user.type(screen.getByRole("textbox", { name: "账号内容" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(submit).toHaveBeenCalledWith(expect.objectContaining({ scope: "local-export" }));
});
