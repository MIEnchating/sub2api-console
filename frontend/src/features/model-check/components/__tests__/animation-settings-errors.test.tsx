import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

it.each(["动画超时", "前置检测超时"])(
  "保存%s且超时无效时显示关联字段错误，不重复弹出 toast",
  async (action) => {
    const errorToast = vi.spyOn(toast, "error");
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    clients.push(client);
    client.setQueryDefaults(["detection-timeout"], { staleTime: Infinity });
    client.setQueryData(["detection-timeout", "animation"], { timeout_seconds: 120, version: 0 });
    client.setQueryData(["detection-timeout", "precheck"], { timeout_seconds: 120, version: 0 });
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
    fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
    if (action.startsWith("前置检测"))
      fireEvent.click(screen.getByRole("tab", { name: "前置检测" }));
    fireEvent.change(screen.getByRole("spinbutton", { name: `${action}（秒）` }), {
      target: { value: "1" },
    });
    fireEvent.click(screen.getByRole("button", { name: `保存${action}` }));
    expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveTextContent("gpt-6-astra");
    const timeout = screen.getByRole("spinbutton", { name: `${action}（秒）` });
    await waitFor(() => expect(timeout).toHaveAttribute("aria-invalid", "true"));
    expect(timeout).toHaveAccessibleDescription(expect.stringContaining("超时不能小于 5 秒"));
    expect(errorToast).not.toHaveBeenCalled();
  },
);
