import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { AnimationPromptDialog } from "../animation-prompt-dialog";

function settings(version: number) {
  return {
    prompt: "旧提示词",
    version,
    default_id: "default",
    templates: [{ id: "default", name: "默认提示词", prompt: "旧提示词" }],
  };
}
const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});
function setup(): ReturnType<typeof render> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationPromptDialog />
    </QueryClientProvider>,
  );
}

it("保存提示词写入后端，重新挂载后读取同一份配置", async () => {
  let stored = settings(3);
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") {
        expect(init.credentials).toBe("include");
        const input = JSON.parse(String(init.body)) as typeof stored;
        writes.push(input);
        stored = { ...input, version: input.version + 1 };
      }
      return Response.json(stored);
    }),
  );
  const user = userEvent.setup();
  const view = setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  const input = await screen.findByRole("textbox", { name: "动画提示词" });
  expect(input).toHaveValue("旧提示词");
  await user.clear(input);
  await user.type(input, "生成蓝色鲸鱼动画");
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(writes).toEqual([
    {
      templates: [{ id: "default", name: "默认提示词", prompt: "生成蓝色鲸鱼动画" }],
      default_id: "default",
      version: 3,
    },
  ]);
  view.unmount();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  expect(await screen.findByRole("textbox", { name: "动画提示词" })).toHaveValue(
    "生成蓝色鲸鱼动画",
  );
});

it("读取未完成时不能保存且可通过键盘取消", async () => {
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((_resolve, reject) => {
        signal?.addEventListener("abort", () => reject(signal?.reason), { once: true });
      });
    }),
  );
  const user = userEvent.setup();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  expect(await screen.findByRole("status", { name: "正在读取动画提示词" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存提示词" })).not.toBeInTheDocument();
  screen.getByRole("button", { name: "取消" }).focus();
  await user.keyboard("{Enter}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(signal?.aborted).toBe(true);
});

it("保存失败保留输入并允许重试，不能显示保存成功", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT")
        return Response.json({ error: "保存失败，请重试" }, { status: 503 });
      return Response.json(settings(1));
    }),
  );
  const user = userEvent.setup();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  fireEvent.change(await screen.findByRole("textbox", { name: "动画提示词" }), {
    target: { value: "新提示词" },
  });
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存提示词" })).toBeEnabled());
  expect(screen.getByRole("textbox", { name: "动画提示词" })).toHaveValue("新提示词");
  expect(screen.queryByText("动画提示词已保存")).not.toBeInTheDocument();
});

it("空白提示词显示字段错误且不发送保存请求", async () => {
  const fetcher = vi.fn(async () => Response.json(settings(1)));
  vi.stubGlobal("fetch", fetcher);
  const user = userEvent.setup();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  const input = await screen.findByRole("textbox", { name: "动画提示词" });
  await user.clear(input);
  await user.type(input, "   ");
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  expect(await screen.findByText("请输入动画提示词")).toBeVisible();
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("HTTP 环境新增第二套提示词并用键盘设为默认后保存全部配置", async () => {
  vi.stubGlobal("crypto", {
    getRandomValues: crypto.getRandomValues.bind(crypto),
    randomUUID: undefined,
  });
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") {
        writes.push(JSON.parse(String(init.body)));
        return Response.json({ ...JSON.parse(String(init.body)), version: 2 });
      }
      return Response.json(settings(1));
    }),
  );
  const user = userEvent.setup();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  await screen.findByRole("textbox", { name: "动画提示词" });
  expect(screen.getByRole("button", { name: "删除提示词" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "新增提示词" }));
  const second = within(screen.getByRole("group", { name: "提示词 2" }));
  await user.type(second.getByRole("textbox", { name: "名称" }), "鲸鱼");
  await user.type(second.getByRole("textbox", { name: "动画提示词" }), "生成鲸鱼动画");
  const radio = second.getByRole("radio", { name: "设为默认" });
  radio.focus();
  await user.keyboard(" ");
  expect(radio).toBeChecked();
  expect(second.getByRole("button", { name: "删除提示词" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual({
    version: 1,
    default_id: expect.any(String),
    templates: [
      settings(1).templates[0],
      { id: expect.any(String), name: "鲸鱼", prompt: "生成鲸鱼动画" },
    ],
  });
  const payload = writes[0] as ReturnType<typeof settings>;
  expect(payload.default_id).toBe(payload.templates[1]?.id);
});

it("名称重复时阻止保存，删除重复的非默认项后可以保存", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") writes.push(JSON.parse(String(init.body)));
      return Response.json(settings(1));
    }),
  );
  const user = userEvent.setup();
  setup();
  await user.click(screen.getByRole("button", { name: "编辑动画提示词" }));
  await screen.findByRole("textbox", { name: "动画提示词" });
  await user.click(screen.getByRole("button", { name: "新增提示词" }));
  const second = within(screen.getByRole("group", { name: "提示词 2" }));
  await user.type(second.getByRole("textbox", { name: "名称" }), "默认提示词");
  await user.type(second.getByRole("textbox", { name: "动画提示词" }), "鲸鱼");
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  expect(await screen.findByText("提示词名称不能重复")).toBeVisible();
  expect(second.getByRole("textbox", { name: "名称" })).toHaveAttribute("aria-invalid", "true");
  expect(writes).toHaveLength(0);
  await user.click(second.getByRole("button", { name: "删除提示词" }));
  await user.click(screen.getByRole("button", { name: "保存提示词" }));
  await waitFor(() => expect(writes).toHaveLength(1));
});
