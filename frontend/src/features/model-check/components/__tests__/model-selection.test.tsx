import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { createConsoleQueryClient } from "@/lib/query-client";
import { RegularCheckPanel } from "../regular-check-panel";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

async function selectModelThenAnotherAccount(): Promise<(response: Response) => void> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = createConsoleQueryClient();
  clients.push(client);
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "主账号", groups: [], platform: "openai" },
      { id: "42", name: "备用账号", groups: [], platform: "openai" },
    ],
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-5.6-sol", "gpt-5.6-luna"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-5.6-sol"] });
  let resolveModels!: (response: Response) => void;
  vi.stubGlobal("fetch", () => new Promise<Response>((resolve) => (resolveModels = resolve)));
  render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 主账号/ }));
  fireEvent.click(await screen.findByRole("checkbox", { name: /gpt-5.6-sol/ }));
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 备用账号/ }));
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  return resolveModels;
}

it("新增账号的模型异步读取完成后保留仍然共同支持的已选模型", async () => {
  const resolveModels = await selectModelThenAnotherAccount();

  await act(async () => resolveModels(Response.json({ models: ["gpt-5.6-sol"] })));

  expect(await screen.findByRole("checkbox", { name: /gpt-5.6-sol/ })).toBeChecked();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeEnabled();
});

it("新增账号的模型读取失败后取消该账号时恢复原有模型选择", async () => {
  const resolveModels = await selectModelThenAnotherAccount();

  await act(async () =>
    resolveModels(Response.json({ detail: "模型接口暂时不可用" }, { status: 502 })),
  );
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 备用账号/ }));

  expect(await screen.findByRole("checkbox", { name: /gpt-5.6-sol/ })).toBeChecked();
});

it("新增账号读取成功但不支持原模型时清除失效选择并禁止提交", async () => {
  const resolveModels = await selectModelThenAnotherAccount();

  await act(async () => resolveModels(Response.json({ models: ["gpt-5.6-luna"] })));
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 备用账号/ }));

  expect(await screen.findByRole("checkbox", { name: /gpt-5.6-sol/ })).not.toBeChecked();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
});
