import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";

import { api, type ModelCheckConfiguration } from "@/api";

import { ModelCheckConfigurationDialog } from "../model-check-configuration-dialog";

const configuration: ModelCheckConfiguration = {
  active: {
    id: "profile-current",
    status: "published",
    note: "当前画像",
    fingerprint: "0123456789abcdef0123456789abcdef",
    created_at: "2026-09-05T00:00:00Z",
    created_by: "operator",
    published_at: "2026-09-05T00:00:00Z",
    payload: {
      claude_profiles: {
        "claude-opus-5": {
          identity_group: ["claude-opus-5"],
          candidate_models: ["claude-opus-5", "claude-sonnet-5"],
          thresholds: [-1, 0, 1, 0.5],
          score_bands: [-2, -1, 0, -2, -1, 0],
          probes: [
            {
              id: "choice-1",
              kind: "choice",
              question: "选择答案",
              options: ["甲", "乙", "丙"],
              weights: { o0: [0, -1] },
            },
          ],
        },
      },
      sol_profile: {
        candidate_models: ["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"],
        quick: [
          {
            id: "number-1",
            kind: "numeric",
            question: "数值是___。",
            clusters: [{ id: "c0", center: 1 }],
            tolerance: { value: 0, mode: "absolute" },
            weights: { c0: [0, -1, -1] },
          },
        ],
        reserve: [],
        thresholds: {
          quick: {
            sol_accept_min: 0.6,
            non_sol_accept_max: 0.4,
            subtype_accept_min: 0.6,
            min_coverage: 0.6,
            min_evidence_coverage: 0.5,
          },
          full: {
            sol_accept_min: 0.6,
            non_sol_accept_max: 0.4,
            subtype_accept_min: 0.6,
            min_coverage: 0.6,
            min_evidence_coverage: 0.5,
          },
        },
      },
    },
  },
  draft: null,
  history: [],
};

const clients: QueryClient[] = [];

function renderDialog(initial = configuration) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  clients.push(client);
  client.setQueryData(["model-check-configuration"], initial);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <ModelCheckConfigurationDialog open onOpenChange={() => undefined} />
    </QueryClientProvider>,
  );
  return client;
}

describe("模型检测规则管理弹窗", () => {
  it("首次打开显示两个系列的生效规则和题目，JSON 编辑默认收起", () => {
    renderDialog();

    expect(screen.getByRole("dialog", { name: "检测规则与题库" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "规则与题目" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("button", { name: "claude-opus-5" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "gpt-5.6-sol" })).toBeVisible();
    expect(screen.getByRole("button", { name: "gpt-5.6-luna" })).toBeVisible();
    expect(screen.getByRole("button", { name: "gpt-5.6-terra" })).toBeVisible();
    expect(screen.queryByText("选择答案")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "判定标准" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("回答特征匹配下限")).toBeVisible();
    expect(screen.queryByRole("textbox", { name: "规则与题库 JSON" })).not.toBeInTheDocument();
  });

  it("输入无效 JSON 时显示字段错误且不提交", async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole("tab", { name: "高级设置" }));
    const editor = await screen.findByRole(
      "textbox",
      { name: "规则与题库 JSON" },
      { timeout: 10_000 },
    );

    await user.click(editor);
    await user.keyboard("{Control>}a{/Control}");
    await user.paste("{");
    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("配置必须是有效的 JSON 对象");
  }, 15_000);

  it("保存规则失败时保留输入并允许重试，只显示一次悬浮错误", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ detail: "配置版本冲突，请重新读取" }, { status: 409 })),
    );
    renderDialog();
    await userEvent.click(screen.getByRole("tab", { name: "高级设置" }));
    const note = screen.getByRole("textbox", { name: "版本说明" });
    fireEvent.change(note, { target: { value: "保留这次修改" } });
    await userEvent.click(screen.getByRole("button", { name: "保存草稿" }));
    expect(await screen.findByText("配置版本冲突，请重新读取")).toBeVisible();
    expect(screen.getAllByText("配置版本冲突，请重新读取")).toHaveLength(1);
    expect(note).toHaveValue("保留这次修改");
    expect(screen.getByRole("button", { name: "保存草稿" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "发布生效" })).toBeDisabled();
  });
});

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("规则后台刷新时保留编辑草稿，保存仍校验开始编辑时的指纹", async () => {
  const save = vi.spyOn(api, "saveModelCheckDraft").mockRejectedValue(new Error("配置版本冲突"));
  const client = renderDialog();
  await userEvent.click(screen.getByRole("tab", { name: "高级设置" }));
  fireEvent.change(screen.getByRole("textbox", { name: "版本说明" }), {
    target: { value: "本地草稿" },
  });
  await act(async () =>
    client.setQueryData(["model-check-configuration"], {
      ...configuration,
      active: {
        ...configuration.active,
        id: "profile-next",
        note: "远端更新",
        fingerprint: "next-fingerprint",
      },
    }),
  );
  await screen.findByText("profile-next");
  expect(screen.getByRole("textbox", { name: "版本说明" })).toHaveValue("本地草稿");
  await userEvent.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        expected_fingerprint: configuration.active.fingerprint,
        note: "本地草稿",
      }),
    ),
  );
});

it.each([
  { action: "发布生效", confirm: "确认发布", method: "publishModelCheckDraft" as const },
  { action: "删除草稿", confirm: "确认删除", method: "discardModelCheckDraft" as const },
])("$action 确认期间版本刷新时仍只操作原先确认的草稿", async (fixture) => {
  const request = vi.spyOn(api, fixture.method).mockRejectedValue(new Error("草稿版本已变化"));
  const draft = {
    ...configuration.active,
    id: "draft-original",
    status: "draft" as const,
    fingerprint: "original-draft",
  };
  const initial = { ...configuration, draft };
  const client = renderDialog(initial);
  await userEvent.click(screen.getByRole("tab", { name: "高级设置" }));
  await userEvent.click(screen.getByRole("button", { name: fixture.action }));
  await act(async () =>
    client.setQueryData(["model-check-configuration"], {
      ...initial,
      draft: { ...draft, id: "draft-next", fingerprint: "next-draft" },
    }),
  );
  await screen.findByText("draft-next");
  await userEvent.click(screen.getByRole("button", { name: fixture.confirm }));
  await waitFor(() => expect(request).toHaveBeenCalledWith("original-draft"));
});
it("规则读取中保留关闭入口，尚未进入高级设置时不提供保存操作", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckConfigurationDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status", { name: "正在读取检测规则" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.queryByRole("button", { name: "保存草稿" })).not.toBeInTheDocument();
  expect(within(screen.getByRole("dialog")).getByRole("button", { name: "关闭" })).toBeEnabled();
  view.unmount();
  client.clear();
});

it("恢复历史版本确认期间配置刷新时保留原目标指纹", async () => {
  const restore = vi
    .spyOn(api, "restoreModelCheckVersion")
    .mockRejectedValue(new Error("配置版本已变化"));
  const initial = {
    ...configuration,
    history: [
      {
        ...configuration.active,
        id: "history-1",
        note: "历史版本",
        claude_profiles: 1,
        probe_count: 2,
      },
    ],
  };
  const client = renderDialog(initial);
  await userEvent.click(screen.getByRole("tab", { name: "高级设置" }));
  await userEvent.click(screen.getByRole("button", { name: "版本历史" }));
  await userEvent.click(
    within(screen.getByRole("dialog", { name: "版本历史" })).getByRole("button", {
      name: "恢复为草稿",
    }),
  );
  await screen.findByRole("dialog", { name: "恢复历史规则" });
  await act(async () =>
    client.setQueryData(["model-check-configuration"], {
      ...initial,
      active: { ...configuration.active, id: "profile-next", fingerprint: "next-fingerprint" },
    }),
  );
  await screen.findByText("profile-next");
  await userEvent.click(
    within(screen.getByRole("dialog", { name: "恢复历史规则" })).getByRole("button", {
      name: "恢复为草稿",
    }),
  );
  await waitFor(() =>
    expect(restore).toHaveBeenCalledWith(
      "history-1",
      configuration.active.fingerprint,
      "恢复自版本 history-1",
    ),
  );
});
