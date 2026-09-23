import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountModelSyncDialog } from "../account-model-sync-dialog";

afterEach(() => vi.unstubAllGlobals());

function setup(options: { ungrouped?: boolean; empty?: boolean; ordered?: boolean } = {}): {
  submissions: unknown[];
  discoveries: string[][];
  close: ReturnType<typeof vi.fn>;
} {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const submissions: unknown[] = [];
  const discoveries: string[][] = [];
  const close = vi.fn();
  let selectedIds: string[] = [];
  const accounts = [
    {
      account_id: "41",
      account_name: "A账号",
      models: ["common", "a-only"],
      enabled_models: ["common"],
    },
    {
      account_id: "42",
      account_name: "B账号",
      models: ["common", "b-only"],
      enabled_models: ["common"],
    },
    { account_id: "43", account_name: "共享账号", models: ["common"], enabled_models: ["common"] },
    { account_id: "44", account_name: "C账号", models: ["common"], enabled_models: ["common"] },
  ];
  const task = {
    id: "discovery",
    status: "succeeded",
    progress: 100,
    result: {
      items: accounts.map((account) => ({ account_id: account.account_id, status: "succeeded" })),
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.includes("dictionaries"))
        return Response.json({
          items: options.ordered
            ? [
                { value: "3", name: "C", enabled: true },
                { value: "2", name: "B", enabled: true },
                { value: "1", name: "A", enabled: true },
              ]
            : [],
        });
      if (path.endsWith("/models/discover")) {
        selectedIds = (JSON.parse(String(init?.body)) as { account_ids: string[] }).account_ids;
        discoveries.push(selectedIds);
        return Response.json(task);
      }
      if (path.endsWith("/tasks/discovery"))
        return Response.json({
          ...task,
          result: { items: selectedIds.map((account_id) => ({ account_id, status: "succeeded" })) },
        });
      if (path.endsWith("/models/preview"))
        return Response.json({
          fingerprint: "catalog",
          account_count: 4,
          accounts_with_catalog: 4,
          blocked_models: [],
          blocked_patterns: [],
          models: [],
          accounts: accounts
            .filter((account) => selectedIds.includes(account.account_id))
            .map((account) => ({
              ...account,
              platform: "openai",
              probe_model: "",
            })),
        });
      if (path.endsWith("/models/apply")) {
        submissions.push(JSON.parse(String(init?.body)));
        return Response.json({ ...task, id: "apply" });
      }
      return Response.json(task);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AccountModelSyncDialog
        open
        accountIds={options.empty ? [] : ["41", "42", "43", "44"]}
        accountPlatforms={new Map(accounts.map((account) => [account.account_id, "openai"]))}
        accountGroups={
          options.ungrouped
            ? new Map()
            : new Map([
                ["41", ["A"]],
                ["42", ["B"]],
                ["43", ["A", "B"]],
                ["44", ["C"]],
              ])
        }
        onOpenChange={close}
        onCompleted={() => undefined}
      />
    </QueryClientProvider>,
  );
  return { submissions, discoveries, close };
}

async function selectGroups(groups: string[]): Promise<void> {
  await userEvent.click(await screen.findByRole("combobox", { name: "同步分组" }));
  for (const group of groups) await userEvent.click(screen.getByRole("option", { name: group }));
  await userEvent.keyboard("{Escape}");
}

it("打开同步模型时先选择分组，确认前不创建模型发现任务", async () => {
  const { discoveries, close } = setup();
  expect(await screen.findByRole("combobox", { name: "同步分组" })).toBeVisible();
  expect(screen.getByRole("button", { name: "开始同步" })).toBeDisabled();
  await selectGroups(["A · 2", "B · 2"]);
  expect(screen.getByText("已选 2 个分组，共 3 个账号")).toBeVisible();
  expect(discoveries).toEqual([]);
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(close).toHaveBeenCalledWith(false);
  expect(discoveries).toEqual([]);
});

it("先选择多个分组再开始同步时只读取选中账号，共享账号只执行一次", async () => {
  const { discoveries, submissions } = setup();
  await selectGroups(["A · 2", "B · 2"]);
  await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
  expect(await screen.findByRole("checkbox", { name: "同步模型 common" })).toBeChecked();
  expect(discoveries).toEqual([["41", "42", "43"]]);
  expect(screen.queryByRole("tab", { name: "C · 1" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "组合分组" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "同步 3 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["common"] },
          { account_id: "42", models: ["common"] },
          { account_id: "43", models: ["common"] },
        ],
        catalog_fingerprint: "catalog",
        probe_models: [],
      },
    ]),
  );
});

it("只选择一个分组也能开始同步，取消该分组后禁用开始按钮", async () => {
  const { discoveries } = setup();
  await selectGroups(["C · 1"]);
  expect(screen.getByRole("button", { name: "开始同步" })).toBeEnabled();
  await selectGroups(["C · 1"]);
  expect(screen.getByRole("button", { name: "开始同步" })).toBeDisabled();
  await selectGroups(["C · 1"]);
  await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
  await screen.findByRole("checkbox", { name: "同步模型 common" });
  expect(discoveries).toEqual([["44"]]);
});

it("账号没有分组时可显式选择未分组后开始同步", async () => {
  const { discoveries } = setup({ ungrouped: true });
  await selectGroups(["未分组 · 4"]);
  await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
  await screen.findByRole("checkbox", { name: "同步模型 common" });
  expect(discoveries).toEqual([["41", "42", "43", "44"]]);
});

it("没有可同步账号时显示空状态且不能创建任务", async () => {
  const { discoveries } = setup({ empty: true });
  expect(await screen.findByText("当前没有可同步的账号")).toBeVisible();
  expect(screen.getByRole("button", { name: "开始同步" })).toBeDisabled();
  expect(discoveries).toEqual([]);
});

it("分组选择关闭后重新打开时清空上次选择，仍需明确开始同步", async () => {
  const discoveries: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith("/models/discover")) discoveries.push(init?.body);
      return Response.json({ items: [] });
    }),
  );
  function Host() {
    const [open, setOpen] = useState(true);
    return (
      <>
        <button onClick={() => setOpen(true)}>打开同步</button>
        <AccountModelSyncDialog
          open={open}
          accountIds={["41"]}
          accountPlatforms={new Map()}
          accountGroups={new Map([["41", ["A"]]])}
          onOpenChange={setOpen}
          onCompleted={() => undefined}
        />
      </>
    );
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Host />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("button", { name: "选择全部分组" }));
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  await userEvent.click(screen.getByRole("button", { name: "打开同步" }));
  expect(screen.getByRole("button", { name: "开始同步" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "同步分组" })).toHaveTextContent("选择要同步的分组");
  expect(discoveries).toEqual([]);
});

it("分组字典顺序改变时按后台顺序展示，键盘确认后才创建任务", async () => {
  const { discoveries } = setup({ ordered: true });
  const trigger = await screen.findByRole("combobox", { name: "同步分组" });
  trigger.focus();
  await userEvent.keyboard("{Enter}");
  await waitFor(() =>
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      "C · 1",
      "B · 2",
      "A · 2",
    ]),
  );
  const option = screen.getByRole("option", { name: "C · 1" });
  await userEvent.click(option);
  expect(option).toHaveAttribute("aria-selected", "true");
  await userEvent.keyboard("{Escape}");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(discoveries).toEqual([]);
  screen.getByRole("button", { name: "开始同步" }).focus();
  await userEvent.keyboard("{Enter}");
  await screen.findByRole("checkbox", { name: "同步模型 common" });
  expect(discoveries).toEqual([["44"]]);
});
