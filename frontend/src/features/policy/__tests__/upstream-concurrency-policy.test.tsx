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

describe("上游超额自动下调", () => {
  it("未配置时默认关闭，并说明完全模式执行、独立下调及恢复条件", () => {
    render(<Editor />);

    const card = screen.getByRole("region", { name: "上游超额自动下调" });
    expect(within(card).getByRole("switch", { name: "启用上游超额自动下调" })).not.toBeChecked();
    expect(card).toHaveTextContent("完全模式");
    expect(card).toHaveTextContent("按调度策略");
    expect(card).toHaveTextContent("暂停低优先级账号");
    expect(card).toHaveTextContent("不受全局并发预算");
    expect(card).toHaveTextContent("不自动扩容或恢复账号");
    expect(card).toHaveTextContent("并发上限自动执行");
    expect(card).toHaveTextContent("调度状态自动执行");
  });

  it("智能扩容和通用自动写入关闭时启用独立下调，仅保存独立开关", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Editor onChange={onChange} />);

    await user.click(screen.getByRole("switch", { name: "启用上游超额自动下调" }));

    expect(screen.getByRole("switch", { name: "启用上游超额自动下调" })).toBeChecked();
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
    const toggle = screen.getByRole("switch", { name: "启用上游超额自动下调" });
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

    await user.click(screen.getByRole("switch", { name: "启用上游超额自动下调" }));

    expect(screen.getByRole("region", { name: "上游超额自动下调" })).toHaveTextContent(
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
