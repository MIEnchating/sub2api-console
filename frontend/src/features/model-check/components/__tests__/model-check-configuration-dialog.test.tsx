import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ModelCheckConfiguration } from "@/api";

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

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(["model-check-configuration"], configuration);
  render(
    <QueryClientProvider client={client}>
      <ModelCheckConfigurationDialog open onOpenChange={() => undefined} />
    </QueryClientProvider>,
  );
}

describe("模型检测画像管理弹窗", () => {
  it("展示生效版本、题目统计和草稿发布状态", () => {
    renderDialog();

    expect(screen.getByRole("dialog", { name: "检测题库与行为画像" })).toBeVisible();
    expect(screen.getByText("profile-current", { selector: "p" })).toBeVisible();
    expect(screen.getByText("1 个")).toBeVisible();
    expect(screen.getByText("2 道")).toBeVisible();
    expect(screen.getByRole("button", { name: "发布生效" })).toBeDisabled();
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "题库与画像 JSON",
    });
    expect(editor.value).toContain('"claude_profiles"');
  });

  it("输入无效 JSON 时显示字段错误且不提交", async () => {
    const user = userEvent.setup();
    renderDialog();
    const editor = screen.getByRole("textbox", { name: "题库与画像 JSON" });

    fireEvent.change(editor, { target: { value: "{" } });
    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("配置必须是有效的 JSON 对象");
  });
});

afterEach(() => vi.unstubAllGlobals());
it("画像配置读取中提供具名轻量反馈并禁止保存", () => {
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
  expect(screen.getByRole("status", { name: "正在读取画像配置" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.getByRole("button", { name: "保存草稿" })).toBeDisabled();
  view.unmount();
  client.clear();
});
