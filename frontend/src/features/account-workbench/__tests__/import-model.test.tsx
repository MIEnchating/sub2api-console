import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster } from "sonner";
import { ImportPanel } from "../components/import-panel";
import { templateKeys } from "../constants";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.unstubAllGlobals();
});
function mount(): ReturnType<typeof render> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(templateKeys.library, { revision: 0, preferred_id: "", items: [] });
  return render(
    <QueryClientProvider client={client}>
      <ImportPanel onStarted={() => undefined} />
      <Toaster />
    </QueryClientProvider>,
  );
}
it("保存检测模型不要求账号资料，重新打开后使用保存值发送预览", async () => {
  let saved = { model: "saved-model", revision: 1 };
  const writes: unknown[] = [];
  let previewBody: unknown;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input, init) => {
      if (String(input).endsWith("/import-model")) {
        if (init?.method === "PUT") {
          const body = JSON.parse(String(init.body));
          writes.push(body);
          saved = { model: body.model, revision: 2 };
        }
        return Response.json(saved);
      }
      previewBody = JSON.parse(String(init?.body));
      return Response.json({
        id: "preview",
        revision: 1,
        action: "import",
        check: true,
        items: [],
        errors: [],
        duplicate_count: 0,
      });
    }),
  );
  const view = mount();
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("saved-model"),
  );
  await user.clear(screen.getByRole("textbox", { name: "检测模型" }));
  await user.type(screen.getByRole("textbox", { name: "检测模型" }), "chosen-model");
  await user.click(screen.getByRole("button", { name: "保存检测模型" }));
  await waitFor(() => expect(writes).toEqual([{ model: "chosen-model", revision: 1 }]));
  expect(screen.queryByText("请粘贴账号资料或上传文件")).not.toBeInTheDocument();
  view.unmount();
  mount();
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("chosen-model"),
  );
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await waitFor(() => expect(previewBody).toMatchObject({ model: "chosen-model" }));
});
it("读取检测模型失败时禁止保存和提交，重试后恢复已保存模型", async () => {
  let fail = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      fail
        ? Response.json({ error: "模型设置读取失败" }, { status: 500 })
        : Response.json({ model: "recovered-model", revision: 3 }),
    ),
  );
  mount();
  expect(screen.getByRole("button", { name: "保存检测模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  fail = false;
  await userEvent.setup().click(retry);
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("recovered-model"),
  );
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
});
it("保存失败保留输入并可重试，空模型显示字段错误且不发送保存", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input, init) => {
      if (init?.method === "PUT") {
        writes.push(JSON.parse(String(init.body)));
        return Response.json({ error: "保存失败" }, { status: 500 });
      }
      return Response.json({ model: "original-model", revision: 1 });
    }),
  );
  mount();
  const user = userEvent.setup();
  const input = screen.getByRole("textbox", { name: "检测模型" });
  await waitFor(() => expect(input).toHaveValue("original-model"));
  await user.clear(input);
  await user.click(screen.getByRole("button", { name: "保存检测模型" }));
  expect(await screen.findByText("请输入检测模型")).toBeVisible();
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(writes).toHaveLength(0);
  await user.type(input, "new-model");
  await user.click(screen.getByRole("button", { name: "保存检测模型" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存检测模型" })).toBeEnabled());
  expect(input).toHaveValue("new-model");
});

it("导入任务启动后清空账号资料并恢复已保存的默认模型", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input) => {
      if (String(input).endsWith("/import-model"))
        return Response.json({ model: "saved-model", revision: 4 });
      if (String(input).endsWith("/preview"))
        return Response.json({
          id: "preview",
          revision: 1,
          action: "import",
          check: true,
          items: [{ id: "one", index: 0, kind: "refresh_token", name: "", email: "" }],
          errors: [],
          duplicate_count: 0,
        });
      return Response.json({ id: "run-one" });
    }),
  );
  mount();
  const user = userEvent.setup();
  const model = screen.getByRole("textbox", { name: "检测模型" });
  await waitFor(() => expect(model).toHaveValue("saved-model"));
  await user.clear(model);
  await user.type(model, "temporary-model");
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await user.click(await screen.findByRole("button", { name: "确认并开始" }));
  await waitFor(() => expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveValue(""));
  expect(model).toHaveValue("saved-model");
});
