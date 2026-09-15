import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

it.each(["开始检测（1 个账号）", "前置检测（1）"])(
  "点击%s且模型和超时无效时显示关联字段错误，不重复弹出 toast",
  async (action) => {
    const errorToast = vi.spyOn(toast, "error");
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    clients.push(client);
    client.setQueryData(
      ["accounts"],
      [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
    );
    client.setQueryData(["model-animation", "schedules"], []);
    client.setQueryData(["model-animation", "history"], []);
    render(
      <QueryClientProvider client={client}>
        <AnimationCheckPanel />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "选择前 20 个账号" }));
    fireEvent.change(screen.getByRole("spinbutton", { name: "请求超时（秒）" }), {
      target: { value: "1" },
    });
    fireEvent.click(screen.getByRole("button", { name: action }));
    const model = screen.getByRole("combobox", { name: "检测模型" });
    const timeout = screen.getByRole("spinbutton", { name: "请求超时（秒）" });
    await waitFor(() => expect(model).toHaveAccessibleDescription("请输入模型 ID"));
    expect(timeout).toHaveAccessibleDescription("超时不能小于 5 秒");
    const modelField = screen.getByRole("group", { name: "检测模型设置" });
    expect(within(modelField).getByRole("alert")).toHaveTextContent("请输入模型 ID");
    expect(modelField).toHaveClass("min-w-0");
    expect(errorToast).not.toHaveBeenCalled();
  },
);
