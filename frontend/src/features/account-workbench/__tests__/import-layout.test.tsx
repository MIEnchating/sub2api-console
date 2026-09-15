import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkbenchImport } from "../components/workbench-import";

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
function mount(element: ReactElement = <WorkbenchImport />): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}

it("账号输入在窄容器纵排，宽容器与配置并排且底部操作可以换行", async () => {
  mount();
  const content = await screen.findByRole("region", { name: "账号内容与文件" });
  const options = screen.getByRole("region", { name: "导入选项" });
  expect(screen.getByRole("form", { name: "账号导入输入" })).toHaveClass("@container/import");
  expect(screen.getByRole("group", { name: "导入内容与选项" })).toHaveClass(
    "grid",
    "min-w-0",
    "@3xl/import:grid-cols-[minmax(0,1fr)_20rem]",
  );
  expect(content.querySelector('[data-slot="json-editor"]')).toHaveClass("h-64");
  expect(within(options).getByRole("combobox", { name: "配置模板" })).toBeInTheDocument();
  expect(screen.getByRole("group", { name: "账号输入操作" })).toHaveClass("flex-wrap");
});

it("默认收起检测模型，通过键盘启用检测后显示输入并保留账号内容", async () => {
  mount();
  const user = userEvent.setup();
  const content = await screen.findByRole("textbox", { name: "账号内容" });
  await user.click(content);
  await user.paste("rt_layout_fixture");
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  const detection = screen.getByRole("checkbox", { name: "导入后检测（会产生模型调用用量）" });
  detection.focus();
  await user.keyboard(" ");
  expect(detection).toHaveAttribute("aria-checked", "true");
  const model = screen.getByRole("textbox", { name: "检测模型" });
  expect(model).toBeEnabled();
  await user.type(model, "gpt-5");
  await user.click(detection);
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  await user.click(detection);
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("gpt-5");
  expect(content).toHaveTextContent("rt_layout_fixture");
});

it("开启检测但未填写模型时显示可修正的模型校验错误", async () => {
  mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_layout_fixture");
  await user.click(screen.getByRole("checkbox", { name: "导入后检测（会产生模型调用用量）" }));
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(await screen.findByText("启用导入后检测时请填写模型")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveAttribute("aria-invalid", "true");
});

it("关闭检测后若模型仍有长度错误，提交时展开输入以便修正", async () => {
  mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_layout_fixture");
  const detection = screen.getByRole("checkbox", { name: "导入后检测（会产生模型调用用量）" });
  await user.click(detection);
  await user.click(screen.getByRole("textbox", { name: "检测模型" }));
  await user.paste("m".repeat(201));
  await user.click(detection);
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(await screen.findByText("模型名称不能超过 200 个字符")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toBeEnabled();
});

it("输入区只保留字段标签，移除重复标题和外围卡片", async () => {
  mount();
  const content = await screen.findByRole("region", { name: "账号内容与文件" });
  expect(within(content).queryByRole("heading", { name: "账号内容" })).not.toBeInTheDocument();
  expect(content).not.toHaveAttribute("data-slot", "card");
  expect(screen.getByRole("region", { name: "导入选项" })).toHaveClass(
    "border-t",
    "@3xl/import:border-t-0",
    "@3xl/import:border-l",
  );
});

it("仅导出模式显示私有文件用途，不提示将写入线上账号", async () => {
  mount(<WorkbenchImport output="export" />);
  await screen.findByRole("combobox", { name: "配置模板" });
  expect(screen.getByText(/生成服务器私有 JSON 文件/)).toBeVisible();
  expect(screen.queryByText(/写入线上托管账号/)).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: /导入后检测/ })).not.toBeInTheDocument();
});

it("私有转换输入自动识别格式，不要求先选择 JSON 或 RT", async () => {
  mount();
  await screen.findByRole("textbox", { name: "账号内容" });
  expect(screen.queryByRole("combobox", { name: "输入格式" })).not.toBeInTheDocument();
});

it("未校验失败时表单不预留空错误行，提交空账号后仍展示字段错误", async () => {
  mount();
  const user = userEvent.setup();
  const input = await screen.findByRole("form", { name: "账号导入输入" });
  expect(input.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveAttribute("aria-invalid", "true");
});

it("解析等待时同时锁定账号内容、侧栏配置和清空入口", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input) =>
      String(input).endsWith("/templates")
        ? Promise.resolve(Response.json([]))
        : new Promise<Response>(() => {}),
    ),
  );
  mount();
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_layout_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await screen.findByText("正在解析账号并读取影响范围");
  expect(screen.getByRole("textbox", { name: "账号内容" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("combobox", { name: "配置模板" })).toBeDisabled();
  const detection = screen.getByRole("checkbox", { name: "导入后检测（会产生模型调用用量）" });
  expect(detection).toHaveAttribute("aria-disabled", "true");
  await user.click(detection);
  expect(detection).toHaveAttribute("aria-checked", "false");
  expect(screen.getByRole("button", { name: "清空输入" })).toBeDisabled();
});

it("本地 JSON 转换占满输入区域且不显示空的管理配置侧栏", () => {
  mount(<WorkbenchImport scope="local-export" output="export" />);
  expect(screen.getByRole("group", { name: "导入内容与选项" })).not.toHaveClass(
    "@3xl/import:grid-cols-[minmax(0,1fr)_20rem]",
  );
  expect(screen.queryByRole("region", { name: "导入选项" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  expect(fetch).not.toHaveBeenCalled();
});
