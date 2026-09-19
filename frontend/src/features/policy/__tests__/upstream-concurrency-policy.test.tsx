import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, policyDraft, policyPayload } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

type PolicyDraft = ReturnType<typeof policyDraft>;

beforeEach(() => {
  // JSDOM 未实现 Base UI 开关转发点击所需的 PointerEvent。
  vi.stubGlobal("PointerEvent", MouseEvent);
});

afterEach(() => vi.unstubAllGlobals());

function initialDraft(): PolicyDraft {
  return policyDraft({
    ...policy,
    auto_apply: { schedulable: false, priority: true, load_factor: true, concurrency: false },
    advanced_policy: {
      ...policy.advanced_policy,
      scaling: { enabled: false, global_max_concurrency: 900, min_per_account: 3 },
    },
  });
}

function Editor(props: {
  draft?: PolicyDraft;
  onChange?: (value: PolicyDraft) => void;
}): ReactElement {
  const [value, setValue] = useState(props.draft ?? initialDraft());
  return (
    <PolicyOperationsEditor
      section="routing"
      value={value}
      onChange={(next) => {
        setValue(next);
        props.onChange?.(next);
      }}
      probesPending={false}
      onProbesEnabledChange={() => undefined}
    />
  );
}

describe("上游共享并发分配", () => {
  it("未配置时默认关闭，并说明完全模式执行、独立分配及恢复条件", () => {
    render(<Editor />);

    const card = screen.getByRole("region", { name: "上游共享并发分配" });
    expect(within(card).getByRole("switch", { name: "启用上游共享并发分配" })).not.toBeChecked();
    expect(card).toHaveTextContent("完全模式");
    expect(card).toHaveTextContent("按调度策略");
    expect(card).toHaveTextContent("暂停低优先级账号");
    expect(card).toHaveTextContent("智能扩容关闭时不应用全局上限");
    expect(card).toHaveTextContent(
      "开启时遵守配置的全局上限、单账号上下限、步长、冷却和自动执行开关",
    );
    expect(card).toHaveTextContent("New API 不受上游共享额度限制");
    expect(card).toHaveTextContent("自动恢复符合健康条件的等待账号");
    expect(card).toHaveTextContent("至少 1 个并发");
    expect(card).toHaveTextContent("不需要开启智能扩容或通用自动执行开关");
  });

  it("智能扩容和通用自动写入关闭时启用独立分配，仅保存独立开关", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Editor onChange={onChange} />);

    await user.click(screen.getByRole("switch", { name: "启用上游共享并发分配" }));

    expect(screen.getByRole("switch", { name: "启用上游共享并发分配" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "启用智能扩容" })).not.toBeChecked();
    expect(screen.getByRole("spinbutton", { name: "全局并发上限" })).toHaveValue(900);
    const saved = policyPayload(onChange.mock.lastCall![0] as PolicyDraft);
    expect(saved?.advanced_policy).toMatchObject({
      upstream_concurrency: { enabled: true },
      scaling: { enabled: false, global_max_concurrency: 900, min_per_account: 3 },
    });
    expect(saved?.auto_apply).toEqual(initialDraft().auto_apply);
  });

  it("读取已启用配置后使用空格关闭，开关状态和保存值同时更新", async () => {
    const user = userEvent.setup();
    const draft = initialDraft();
    draft.advanced_policy.upstream_concurrency = { enabled: true };
    const onChange = vi.fn();
    render(<Editor draft={draft} onChange={onChange} />);
    const toggle = screen.getByRole("switch", { name: "启用上游共享并发分配" });
    expect(toggle).toBeChecked();
    toggle.focus();

    await user.keyboard(" ");

    expect(toggle).not.toBeChecked();
    expect(toggle).toHaveFocus();
    expect(policyPayload(onChange.mock.lastCall![0] as PolicyDraft)?.advanced_policy).toMatchObject(
      {
        upstream_concurrency: { enabled: false },
      },
    );
  });

  it("监控模式可保存独立开关，并明确提示仅预览", async () => {
    const user = userEvent.setup();
    const draft = initialDraft();
    draft.mode = "监控模式";
    const onChange = vi.fn();
    render(<Editor draft={draft} onChange={onChange} />);

    await user.click(screen.getByRole("switch", { name: "启用上游共享并发分配" }));

    expect(screen.getByRole("region", { name: "上游共享并发分配" })).toHaveTextContent(
      "监控模式仅预览",
    );
    expect(policyPayload(onChange.mock.lastCall![0] as PolicyDraft)).toMatchObject({
      mode: "监控模式",
      advanced_policy: { upstream_concurrency: { enabled: true } },
    });
  });

  it("独立开关收到非布尔值时拒绝保存，避免误启用", () => {
    const draft = initialDraft();
    draft.advanced_policy.upstream_concurrency = { enabled: "true" };

    expect(policyPayload(draft)).toBeNull();
  });
});

describe("共享并发账号范围保存", () => {
  it("指定范围保存稳定 ID，空选择不会改为全部账号", () => {
    for (const ids of [["41", "42"], []]) {
      const draft = initialDraft();
      draft.advanced_policy.upstream_concurrency = {
        enabled: true,
        account_mode: "selected",
        account_ids: ids,
      };
      expect(policyPayload(draft)?.advanced_policy?.upstream_concurrency).toEqual({
        enabled: true,
        account_mode: "selected",
        account_ids: ids,
      });
    }
  });
  it.each([
    { account_mode: "invalid" },
    { upstream_ids: "upstream-a" },
    { upstream_ids: [41] },
    { upstream_ids: [" "] },
    { account_ids: "41" },
    { account_ids: [41] },
    { account_ids: ["name"] },
    { account_ids: ["0"] },
  ])("非法账号范围 %j 拒绝保存", (scope) => {
    const draft = initialDraft();
    draft.advanced_policy.upstream_concurrency = { enabled: true, ...scope };
    expect(policyPayload(draft)).toBeNull();
  });
});

it.each([{ ids: ["Upstream-A", "upstream-b"] }, { ids: [] }])(
  "指定上游保存稳定 ID 或空范围 %j",
  ({ ids }) => {
    const draft = initialDraft();
    draft.advanced_policy.upstream_concurrency = {
      enabled: true,
      account_mode: "upstreams",
      upstream_ids: ids,
    };
    expect(policyPayload(draft)?.advanced_policy?.upstream_concurrency).toEqual(
      draft.advanced_policy.upstream_concurrency,
    );
  },
);
