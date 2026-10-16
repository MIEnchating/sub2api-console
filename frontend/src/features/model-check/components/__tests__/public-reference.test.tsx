import { render, screen, within, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PublicReferenceNotice } from "../public-reference-notice";
import { VerificationPublicReference } from "../verification-public-reference";
import { VerificationIdentityStatus, VerificationIdentitySummary } from "../verification-status";
import { VerificationCombinationProgress } from "../verification-combination-progress";

afterEach(cleanup);

it("公开参考未加载时显示局部加载，失败后可以用键盘重试", async () => {
  const retry = vi.fn();
  const view = render(<PublicReferenceNotice models={["gpt-6-astra"]} pending onRetry={retry} />);
  expect(screen.getByRole("status")).toHaveTextContent("正在读取公开参考目录");
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  view.rerender(<PublicReferenceNotice models={["gpt-6-astra"]} pending={false} onRetry={retry} />);
  const button = screen.getByRole("button", { name: "重新读取" });
  button.focus();
  await userEvent.keyboard("{Enter}");
  expect(retry).toHaveBeenCalledOnce();
});

it("未知精确型号和空目录不借用相似型号且长名称可以换行", () => {
  const model = "claude-opus-5-5-" + "long-model-name-".repeat(20);
  render(
    <PublicReferenceNotice models={[model]} profiles={[]} pending={false} onRetry={vi.fn()} />,
  );
  expect(screen.getByText(/该精确型号暂无公开参考/)).toHaveClass("wrap-anywhere");
  expect(screen.queryByText(/额外 \d+ 次请求/)).not.toBeInTheDocument();
});

it("已登记模型显示真实请求预算而缺失分词基线的 Gemini 解释原因", () => {
  render(
    <PublicReferenceNotice
      models={["gpt-6-astra", "gemini-3.7-flash"]}
      profiles={[
        {
          model: "openai/gpt-6-astra",
          aliases: ["gpt-6-astra"],
          requests: 60,
          methods: ["knowledge"],
        },
        {
          model: "google/gemini-3.7-flash",
          aliases: ["gemini-3.7-flash"],
          requests: 0,
          methods: [],
          unavailable: "公开记录未返回用量",
        },
      ]}
      pending={false}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.getByText(/额外 60 次请求 · 知识边界比较/)).toBeVisible();
  expect(screen.getByText(/gemini-3.7-flash：公开记录未返回用量/)).toBeVisible();
});

it("参考显著不符在统一结果突出显示，能力未通过不替代真伪判断", () => {
  const result = {
    public_reference: { version: "v1", status: "mismatch", summary: "知识边界与参考显著不符" },
    capability: { status: "failed" },
    authenticity: { status: "inconclusive" },
  };
  render(
    <>
      <VerificationIdentityStatus result={result} />
      <VerificationIdentitySummary result={result} />
    </>,
  );
  expect(screen.getByText("与公开参考显著不符（疑似冒充）")).toBeVisible();
  expect(screen.getByText("知识边界与参考显著不符")).toBeVisible();
});

it("分词零差异与分布零距离仍明确无法判定且保留来源链接", async () => {
  render(
    <VerificationPublicReference
      result={{
        public_reference: {
          version: "v1",
          status: "inconclusive",
          summary: "仅有差异证据",
          model: "qwen3.7-plus",
          sources: [
            {
              name: "KBF",
              url: "https://github.com/Ooo0ption/KBF/blob/pinned/data.json",
              sha256: "abc",
              revision: "pinned",
              license: "Apache-2.0",
            },
          ],
          tokenizer: {
            status: "consistent",
            samples: [{ id: "中文", reference: 51, observed: 51, status: "consistent" }],
          },
          distribution: {
            status: "evidence_only",
            mean_jsd: 0,
            cells: [{ id: "num10_zh", valid: 30, reference_valid: 30, jsd: 0 }],
          },
        },
      }}
    />,
  );
  expect(screen.getByText("公开参考：无法判定")).toBeVisible();
  const row = screen.getByRole("row", { name: /num10_zh/ });
  expect(within(row).getByText("0.000")).toBeVisible();
  await userEvent.click(screen.getByText("参考来源与校验值"));
  expect(screen.getByRole("link", { name: "KBF" })).toHaveAttribute(
    "href",
    "https://github.com/Ooo0ption/KBF/blob/pinned/data.json",
  );
  expect(screen.getByText(/JSD 为 0～1/)).toBeVisible();
});

it("公开参考采样在同一检测过程单列依据且不计入能力题", () => {
  render(
    <VerificationCombinationProgress
      active
      result={{
        total: 1,
        probes: [
          {
            id: "public_kbf",
            label: "参考题",
            round: 1,
            category: "public_reference",
            status: "observed",
          },
        ],
      }}
    />,
  );
  expect(screen.getByRole("region", { name: "公开参考对比" })).toBeVisible();
  expect(screen.queryByRole("region", { name: "能力检测" })).not.toBeInTheDocument();
});

it("历史报告缺少公开参考字段时不补造本轮结论", () => {
  render(<VerificationPublicReference result={{}} />);
  expect(screen.queryByRole("region", { name: "公开模型指纹对比" })).not.toBeInTheDocument();
});

it("紧凑参考提示始终显示预算，说明与型号目录默认收起且可展开", async () => {
  render(
    <PublicReferenceNotice
      compact
      models={["model-a"]}
      profiles={[{ model: "model-a", aliases: ["model-a"], requests: 12, methods: ["tokenizer"] }]}
      pending={false}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.getByText(/额外 12 次请求/)).toBeVisible();
  expect(screen.getByText("额外请求按上游用量计费")).toBeVisible();
  expect(screen.getByText(/OAuth 账号不能复现参考参数/)).not.toBeVisible();
  await userEvent.click(screen.getByText(/参考说明与型号目录/));
  expect(screen.getByText(/OAuth 账号不能复现参考参数/)).toBeVisible();
  expect(screen.getByText(/model-a · 12 次请求/)).toBeVisible();
});

it("旧知识边界报告有超时请求时展示具体原因与有效覆盖，不只显示泛化结论", () => {
  const result = {
    public_reference: {
      version: "v1",
      status: "inconclusive",
      summary: "旧泛化原因",
      knowledge: {
        status: "inconclusive",
        valid: 404,
        total: 414,
      },
    },
    probes: [
      {
        id: "public_kbf_crypto_params_0",
        category: "public_reference",
        label: "知识边界 · crypto_params · 1",
        status: "error",
        error: "请求超时",
      },
    ],
  };
  render(<VerificationIdentitySummary result={result} />);
  expect(screen.getByText(/1 个知识边界请求失败/)).toHaveTextContent("404/414");
  expect(screen.getByText(/知识边界 · crypto_params · 1：请求超时/)).toBeVisible();
  expect(screen.queryByText("旧泛化原因")).not.toBeInTheDocument();
});
