import { Fragment, StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountModelSyncDialog } from "../account-model-sync-dialog";

afterEach(() => vi.unstubAllGlobals());

async function setup(
  options: {
    empty?: boolean;
    sharedGroup?: boolean;
    strictMode?: boolean;
    failSave?: boolean;
    saveGate?: Promise<void>;
    failRefresh?: boolean;
  } = {},
): Promise<{
  submissions: unknown[];
  settingsWrites: unknown[];
  reopen: () => Promise<void>;
  retry: () => void;
}> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const submissions: unknown[] = [];
  const settingsWrites: unknown[] = [];
  let patterns = ["legacy-*"];
  let saved = false;
  let failSave = options.failSave;
  let failRefresh = options.failRefresh;
  const task = {
    id: "discovery",
    operation: "account-model-discovery",
    status: "succeeded",
    progress: 100,
    result: {
      items: [
        { account_id: "41", status: "succeeded" },
        { account_id: "42", status: "succeeded" },
      ],
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.includes("dictionaries")) return Response.json({ items: [] });
      if (path.endsWith("/config/model-sync")) {
        if (init?.method === "PUT") {
          settingsWrites.push(JSON.parse(String(init.body)));
          await options.saveGate;
          if (failSave)
            return Response.json(
              { error: { message: "配置保存失败", code: "save_failed" } },
              { status: 500 },
            );
          patterns = (JSON.parse(String(init.body)) as { blocked_patterns: string[] })
            .blocked_patterns;
          saved = true;
        }
        return Response.json({ blocked_patterns: patterns });
      }
      if (path.endsWith("/models/preview")) {
        if (saved && failRefresh)
          return Response.json(
            { error: { message: "预览读取失败", code: "read_failed" } },
            { status: 500 },
          );
        return Response.json({
          account_count: 2,
          accounts_with_catalog: 2,
          blocked_models: saved
            ? patterns.filter((pattern) => ["known", "discovered"].includes(pattern))
            : [],
          blocked_patterns: saved ? patterns : [],
          fingerprint: saved ? "blocked-catalog" : "catalog",
          models: [{ model: "known", account_count: 2 }],
          accounts: ["41", "42"].map((id) => ({
            account_id: id,
            account_name: `账号 ${id}`,
            platform: "openai",
            models: ["known", "discovered"],
            enabled_models: options.empty || (options.sharedGroup && id === "42") ? [] : ["known"],
            probe_model: "known",
          })),
        });
      }
      if (path.endsWith("/models/apply")) {
        submissions.push(JSON.parse(String(init?.body)));
        return Response.json({
          ...task,
          id: "apply",
          operation: "account-model-apply",
          result: { probe_disabled: true },
        });
      }
      if (path.endsWith("/tasks/apply"))
        return Response.json({ ...task, id: "apply", result: { probe_disabled: true } });
      return Response.json(task);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = options.strictMode ? StrictMode : Fragment;
  const view = (open: boolean) => (
    <Wrapper>
      <QueryClientProvider client={client}>
        <AccountModelSyncDialog
          open={open}
          accountIds={["41", "42"]}
          accountPlatforms={
            new Map([
              ["41", "openai"],
              ["42", "openai"],
            ])
          }
          accountGroups={
            new Map([
              ["41", ["A"]],
              ["42", [options.sharedGroup ? "A" : "B"]],
            ])
          }
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>
    </Wrapper>
  );
  const rendered = render(view(true));
  async function start(): Promise<void> {
    await userEvent.click(screen.getByRole("button", { name: "选择全部分组" }));
    await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
    await screen.findByRole("textbox", { name: "手动输入同步模型" });
  }
  await start();
  return {
    submissions,
    settingsWrites,
    reopen: async () => {
      rendered.rerender(view(false));
      rendered.rerender(view(true));
      await start();
    },
    retry: () => {
      failSave = false;
      failRefresh = false;
    },
  };
}

it("手动添加模型仅写入当前分组，默认空探活不会被补选", async () => {
  const { submissions } = await setup();
  const input = await screen.findByRole("textbox", { name: "手动输入同步模型" });
  await userEvent.type(input, "custom-model");
  await userEvent.click(screen.getByRole("button", { name: "添加模型" }));
  expect(screen.getByRole("checkbox", { name: "同步模型 custom-model" })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "统一探活模型" })).toHaveTextContent("选择探活模型");
  await userEvent.click(screen.getByRole("tab", { name: "B · 1" }));
  expect(screen.queryByRole("checkbox", { name: "同步模型 custom-model" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["known", "custom-model"], manual_models: ["custom-model"] },
          { account_id: "42", models: ["known"] },
        ],
        catalog_fingerprint: "catalog",
        probe_models: [],
      },
    ]),
  );
  expect(await screen.findByText("模型写入已完成，未执行探活")).toBeVisible();
});

it("右键排除保存到全局规则，跨分组和重新打开同步均不再出现", async () => {
  const { settingsWrites, reopen } = await setup();
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 known" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() =>
    expect(settingsWrites).toEqual([{ blocked_patterns: ["legacy-*", "known"] }]),
  );
  await waitFor(() =>
    expect(screen.queryByRole("checkbox", { name: "同步模型 known" })).not.toBeInTheDocument(),
  );
  await userEvent.click(screen.getByRole("tab", { name: "B · 1" }));
  expect(screen.queryByRole("checkbox", { name: "同步模型 known" })).not.toBeInTheDocument();
  await reopen();
  expect(screen.queryByRole("checkbox", { name: "同步模型 known" })).not.toBeInTheDocument();
});

it("全局屏蔽后手动输入同名模型时显示字段错误，不恢复卡片", async () => {
  await setup();
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 known" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(screen.getByRole("button", { name: "添加模型" })).toBeEnabled());
  await userEvent.type(screen.getByRole("textbox", { name: "手动输入同步模型" }), "KNOWN");
  await userEvent.click(screen.getByRole("button", { name: "添加模型" }));
  expect(
    await screen.findByText("该模型已被全局屏蔽，请先在全局屏蔽模型设置中移除对应规则"),
  ).toBeVisible();
  expect(screen.queryByRole("checkbox", { name: "同步模型 KNOWN" })).not.toBeInTheDocument();
});

it("保存全局屏蔽失败时保留模型与勾选，可以再次排除", async () => {
  const { retry, settingsWrites } = await setup({ failSave: true });
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 known" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(settingsWrites).toHaveLength(1));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
  expect(screen.getByRole("checkbox", { name: "同步模型 known" })).toBeChecked();
  retry();
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 known" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() =>
    expect(screen.queryByRole("checkbox", { name: "同步模型 known" })).not.toBeInTheDocument(),
  );
});

it("全局屏蔽保存期间禁止同步和重复排除", async () => {
  let finish: () => void = () => undefined;
  const saveGate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const { submissions, settingsWrites } = await setup({ saveGate });
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 discovered" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(settingsWrites).toHaveLength(1));
  expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "同步模型 known" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  expect(submissions).toEqual([]);
  finish();
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
});

it("屏蔽已保存但预览刷新失败时保持隐藏，重新读取后才允许同步", async () => {
  const { retry, submissions } = await setup({ failRefresh: true });
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 discovered" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  const reload = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("checkbox", { name: "同步模型 discovered" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeDisabled();
  retry();
  await userEvent.click(reload);
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      expect.objectContaining({ catalog_fingerprint: "blocked-catalog" }),
    ]),
  );
});

it("打开同步窗口时只勾选已添加模型，新发现模型保持未选中", async () => {
  await setup();
  expect(await screen.findByRole("checkbox", { name: "同步模型 known" })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "同步模型 discovered" })).not.toBeChecked();
});

it("普通取消勾选保留模型卡片，仍能再次勾选且不写全局规则", async () => {
  const { settingsWrites } = await setup();
  const checkbox = await screen.findByRole("checkbox", { name: "同步模型 known" });
  await userEvent.click(checkbox);
  expect(checkbox).not.toBeChecked();
  await userEvent.click(checkbox);
  expect(checkbox).toBeChecked();
  expect(settingsWrites).toEqual([]);
});

it("手动输入通配符时显示字段错误且不增加模型", async () => {
  await setup();
  await userEvent.type(await screen.findByRole("textbox", { name: "手动输入同步模型" }), "*");
  await userEvent.click(screen.getByRole("button", { name: "添加模型" }));
  expect(await screen.findByText("请输入不含空白和通配符的具体模型名称")).toBeVisible();
  expect(screen.queryByRole("checkbox", { name: "同步模型 *" })).not.toBeInTheDocument();
});

it("没有已添加模型时保持全不选，并禁用同步直到手动选择", async () => {
  await setup({ empty: true });
  expect(await screen.findByRole("checkbox", { name: "同步模型 known" })).not.toBeChecked();
  const submit = screen.getByRole("button", { name: "同步 2 个账号" });
  expect(submit).toBeDisabled();
  await userEvent.click(screen.getByRole("checkbox", { name: "同步模型 discovered" }));
  expect(submit).toBeEnabled();
});

it("同组账号配置不同模型时提交各自已有模型，不替其他账号自动添加", async () => {
  const { submissions } = await setup({ sharedGroup: true });
  await userEvent.click(await screen.findByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["known"] },
          { account_id: "42", models: [] },
        ],
        catalog_fingerprint: "catalog",
        probe_models: [],
      },
    ]),
  );
});

it("选择探活模型后排除另一模型，提交仍保留所选探活模型", async () => {
  const { submissions } = await setup();
  await userEvent.click(await screen.findByRole("combobox", { name: "统一探活模型" }));
  await userEvent.click(await screen.findByRole("option", { name: "known" }));
  await userEvent.keyboard("{Escape}");
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 discovered" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["known"] },
          { account_id: "42", models: ["known"] },
        ],
        catalog_fingerprint: "blocked-catalog",
        probe_models: ["known"],
      },
    ]),
  );
});

it("屏蔽所选探活模型后移除该探活项，同时保留其他分组勾选和手动模型", async () => {
  const { submissions } = await setup();
  await userEvent.type(screen.getByRole("textbox", { name: "手动输入同步模型" }), "custom");
  await userEvent.click(screen.getByRole("button", { name: "添加模型" }));
  await userEvent.click(screen.getByRole("combobox", { name: "统一探活模型" }));
  await userEvent.click(screen.getByRole("option", { name: "known" }));
  await userEvent.click(screen.getByRole("option", { name: "custom" }));
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("tab", { name: "B · 1" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "同步模型 discovered" }));
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 known" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
  expect(screen.getByRole("tab", { name: "B · 1" })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("checkbox", { name: "同步模型 discovered" })).toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["custom"], manual_models: ["custom"] },
          { account_id: "42", models: ["discovered"] },
        ],
        catalog_fingerprint: "blocked-catalog",
        probe_models: ["custom"],
      },
    ]),
  );
});

it("排除手动模型后也遵守全局屏蔽，不将手动模型写入同步请求", async () => {
  const { submissions } = await setup();
  await userEvent.type(screen.getByRole("textbox", { name: "手动输入同步模型" }), "custom");
  await userEvent.click(screen.getByRole("button", { name: "添加模型" }));
  fireEvent.contextMenu(screen.getByRole("group", { name: "模型 custom" }), { button: 2 });
  await userEvent.click(await screen.findByRole("menuitem", { name: /^排除此模型/ }));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled());
  expect(screen.queryByRole("checkbox", { name: "同步模型 custom" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
  await waitFor(() =>
    expect(submissions).toEqual([
      {
        accounts: [
          { account_id: "41", models: ["known"], manual_models: [] },
          { account_id: "42", models: ["known"] },
        ],
        catalog_fingerprint: "blocked-catalog",
        probe_models: [],
      },
    ]),
  );
});

it("严格模式下模型发现已完成时移除启动提示，并允许关闭同步窗口", async () => {
  await setup({ strictMode: true });
  expect(
    screen.queryByRole("status", { name: "正在创建账号模型发现任务" }),
  ).not.toBeInTheDocument();
  for (const button of screen.getAllByRole("button", { name: "关闭" }))
    expect(button).toBeEnabled();
});
