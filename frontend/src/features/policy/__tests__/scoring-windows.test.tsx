import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, PolicyRulesEditor, policyPayload } from "../../../App";

type Draft = Parameters<typeof PolicyRulesEditor>[0]["value"];
function initialDraft(): Draft {
  return {
    mode: "完全模式",
    global_strategy: "balanced",
    missing_rate_fallback: "current_cost_wall",
    change_threshold: "0.1",
    cooldown_seconds: 60,
    auto_apply: {},
    excluded_group_ids: [],
    traffic_enabled: true,
    probe_interval_seconds: 300,
    probe_model: "",
    traffic_lookback_minutes: 120,
    max_samples_per_account: 60,
    advanced_policy: {
      upstream_multiplier: { interval_seconds: 120 },
      traffic: { refresh_seconds: 60 },
      account_rate_sync: { interval_seconds: 120, batch_size: 0, batch_percent: 0 },
      scoring: { short_window: 10, long_window: 60 },
    },
  };
}
function Editor(props: { onChange: (value: Draft) => void }) {
  const [draft, setDraft] = useState(initialDraft);
  function change(value: Draft): void {
    setDraft(value);
    props.onChange(value);
  }
  return (
    <>
      <PolicyOperationsEditor
        section="sampling"
        value={draft}
        onChange={change}
        probesEnabled
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />
      <PolicyRulesEditor value={draft} onChange={change} />
    </>
  );
}

describe("评分历史与探针新鲜度", () => {
  it("未配置新字段时显示默认值，编辑两项设置保留探测间隔和样本条数", () => {
    const onChange = vi.fn();
    render(<Editor onChange={onChange} />);
    const freshness = screen.getByRole("spinbutton", { name: "当前探针有效期（秒）" });
    const history = screen.getByRole("spinbutton", { name: "评分历史范围（分钟）" });
    expect(freshness).toHaveValue(900);
    expect(history).toHaveValue(1440);
    fireEvent.change(freshness, { target: { value: "1800" } });
    fireEvent.change(history, { target: { value: "720" } });
    expect(freshness).toHaveValue(1800);
    expect(history).toHaveValue(720);
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        probe_interval_seconds: 300,
        advanced_policy: expect.objectContaining({
          probe: expect.objectContaining({ freshness_seconds: 1800 }),
          scoring: expect.objectContaining({
            history_window_minutes: 720,
            short_window: 10,
            long_window: 60,
          }),
        }),
      }),
    );
  });
  it("清空新鲜度字段时允许编辑且阻止提交空值", () => {
    const onChange = vi.fn();
    render(<Editor onChange={onChange} />);
    const input = screen.getByRole("spinbutton", { name: "当前探针有效期（秒）" });
    fireEvent.change(input, { target: { value: "" } });
    expect(input).toHaveValue(null);
    expect(policyPayload(onChange.mock.lastCall![0] as Draft)).toBeNull();
  });
  it.each([
    ["probe", "freshness_seconds", 0],
    ["probe", "freshness_seconds", 86401],
    ["scoring", "history_window_minutes", 1.5],
    ["scoring", "history_window_minutes", 10081],
  ])("%s.%s 输入 %s 时拒绝提交", (section, field, value) => {
    const draft = initialDraft();
    expect(policyPayload(draft)).not.toBeNull();
    draft.advanced_policy[section] = { [field]: value };
    expect(policyPayload(draft)).toBeNull();
  });
});
