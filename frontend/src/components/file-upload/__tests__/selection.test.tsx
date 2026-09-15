import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { FileUpload } from "@/components/file-upload";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  toast.dismiss();
});

it("拖入不支持的格式不会读取，扩展名大小写不影响有效文件", () => {
  const select = vi.fn();
  render(<FileUpload {...base} onSelect={select} />);
  const region = screen.getByRole("group", { name: "文件上传" });
  fireEvent.drop(region, { dataTransfer: { files: [new File(["data"], "accounts.exe")] } });
  expect(select).not.toHaveBeenCalled();
  const accepted = new File(["{}"], "ACCOUNTS.JSON");
  fireEvent.drop(region, { dataTransfer: { files: [accepted] } });
  expect(select).toHaveBeenCalledWith(accepted);
});
const base = { label: "账号文件", accept: ".txt,.json", description: "TXT / JSON，最大 2 MB" };

it("初始状态提供明确选择按钮，键盘可打开文件选择且取消不提交", async () => {
  const select = vi.fn();
  render(<FileUpload {...base} onSelect={select} />);
  expect(screen.getByRole("status")).toHaveTextContent("未选择文件");
  const file = screen.getByLabelText("账号文件");
  const click = vi.spyOn(file, "click");
  const user = userEvent.setup();
  await user.tab();
  expect(screen.getByRole("button", { name: "选择文件" })).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(click).toHaveBeenCalledOnce();
  fireEvent.change(file, { target: { files: [] } });
  expect(select).not.toHaveBeenCalled();
});

it("再次选择同一个文件仍触发读取，原生输入不会持有文件", async () => {
  const select = vi.fn();
  render(<FileUpload {...base} onSelect={select} />);
  const user = userEvent.setup();
  const file = new File(["rt_fixture"], "accounts.txt", { type: "text/plain" });
  const input = screen.getByLabelText("账号文件");
  await user.upload(input, file);
  await user.upload(input, file);
  expect(select.mock.calls).toEqual([[file], [file]]);
  expect(input).toHaveValue("");
});

it("拖入一个文件交给读取回调，拖入多个文件不会悄悄读取第一项", () => {
  const select = vi.fn();
  render(<FileUpload {...base} onSelect={select} />);
  const first = new File(["one"], "one.txt");
  const second = new File(["two"], "two.txt");
  const region = screen.getByRole("group", { name: "文件上传" });
  fireEvent.drop(region, { dataTransfer: { files: [first, second] } });
  expect(select).not.toHaveBeenCalled();
  fireEvent.drop(region, { dataTransfer: { files: [first] } });
  expect(select).toHaveBeenCalledWith(first);
});

it.each([{ disabled: true }, { busy: true }])("禁用或读取中禁止选择和拖入：%o", async (state) => {
  const select = vi.fn();
  render(<FileUpload {...base} {...state} onSelect={select} />);
  expect(screen.getByRole("button")).toBeDisabled();
  expect(screen.getByLabelText("账号文件")).toBeDisabled();
  fireEvent.drop(screen.getByRole("group"), {
    dataTransfer: { files: [new File(["one"], "one.txt")] },
  });
  expect(select).not.toHaveBeenCalled();
  expect(screen.getByRole("group")).toHaveAttribute("aria-busy", String(!!state.busy));
});

it("长文件名不会挤压选择按钮，清空受控文件名后恢复空状态", () => {
  const name = "very-long-account-file-".repeat(20) + ".json";
  const view = render(<FileUpload {...base} fileName={name} onSelect={() => {}} />);
  expect(screen.getByRole("status")).toHaveTextContent(name);
  expect(screen.getByRole("status")).toHaveClass("truncate");
  expect(screen.getByRole("button", { name: "重新选择" })).toHaveClass("shrink-0");
  view.rerender(<FileUpload {...base} onSelect={() => {}} />);
  expect(screen.getByRole("status")).toHaveTextContent("未选择文件");
});

it("悬停文件名时通过共享提示展示完整名称", async () => {
  const user = userEvent.setup();
  const name = "very-long-account-file-".repeat(20) + ".json";
  render(<FileUpload {...base} fileName={name} onSelect={() => {}} />);
  const status = screen.getByRole("status");
  expect(status).not.toHaveAttribute("title");
  await user.hover(status);
  await waitFor(() =>
    expect(document.querySelector('[data-slot="tooltip-content"]')).toHaveTextContent(name),
  );
});
