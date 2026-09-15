import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { WorkbenchMixedForm } from "../components/workbench-mixed-form";
import { WorkbenchImport } from "../components/workbench-import";
import { WorkbenchOAuthBatchForm } from "../components/workbench-oauth-batch-form";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  toast.dismiss();
});

const forms = ["账号输入", "私有转换", "批量授权"] as const;
function mount(kind: (typeof forms)[number]): void {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json([])),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      {kind === "账号输入" && <WorkbenchMixedForm onSubmit={() => {}} />}
      {kind === "私有转换" && <WorkbenchImport scope="local-export" output="export" />}
      {kind === "批量授权" && <WorkbenchOAuthBatchForm onSubmit={() => {}} onClose={() => {}} />}
    </QueryClientProvider>,
  );
}

it.each(forms)("%s读取成功后显示文件名，后续读取失败保留原内容并允许重试", async (kind) => {
  mount(kind);
  const user = userEvent.setup();
  const input = await screen.findByLabelText("从文件读取（最大 2 MB）");
  const content = "operator@example.test";
  const first = new File([content], "first.txt", { type: "text/plain" });
  Object.defineProperty(first, "text", { value: async () => content });
  await user.upload(input, first);
  expect(await screen.findByRole("status", { name: "first.txt" })).toHaveTextContent("first.txt");
  const failed = new File(["replacement"], "failed.txt", { type: "text/plain" });
  Object.defineProperty(failed, "text", {
    value: async () => {
      throw new Error("文件读取中断");
    },
  });
  await user.upload(input, failed);
  await waitFor(() =>
    expect(screen.getByRole("group", { name: "文件上传" })).toHaveAttribute("aria-busy", "false"),
  );
  expect(
    within(screen.getByRole("group", { name: "文件上传" })).getByRole("status"),
  ).toHaveTextContent("first.txt");
  expect(screen.getByRole("button", { name: "重新选择" })).toBeEnabled();
  const editor = await screen.findByRole("textbox", {
    name: kind === "批量授权" ? "批量授权内容" : "账号内容",
  });
  if (editor instanceof HTMLTextAreaElement) expect(editor).toHaveValue(content);
  else expect(editor).toHaveTextContent(content);
});

it.each(forms)("%s拒绝超过2MB的文件，不读取文件且保留选择入口", async (kind) => {
  mount(kind);
  const file = new File(["oversized"], "large.txt", { type: "text/plain" });
  const read = vi.fn(async () => "oversized");
  Object.defineProperty(file, "size", { value: 2 * 1024 * 1024 + 1 });
  Object.defineProperty(file, "text", { value: read });
  await userEvent.setup().upload(await screen.findByLabelText("从文件读取（最大 2 MB）"), file);
  expect(read).not.toHaveBeenCalled();
  expect(
    within(screen.getByRole("group", { name: "文件上传" })).getByRole("status"),
  ).toHaveTextContent("未选择文件");
  expect(screen.getByRole("button", { name: "选择文件" })).toBeEnabled();
});
