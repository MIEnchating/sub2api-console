import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { PolicyScopeEditor } from "@/App";

type Draft = Parameters<typeof PolicyScopeEditor>[0]["value"];

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function ManagedGroups(props: { selected: string[]; onChange: (value: Draft) => void }) {
  const [draft, setDraft] = useState<Draft>({
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
      scope: { managed_group_mode: "selected", managed_group_ids: props.selected },
    },
  });
  return (
    <PolicyScopeEditor
      value={draft}
      onChange={(value) => {
        setDraft(value);
        props.onChange(value);
      }}
      groups={[
        {
          id: "6",
          name: "主力组",
          platforms: ["openai"],
          strategy: "balanced",
          strategy_source: "global_default",
          participation_status: "participating",
          participation_reason: null,
          account_count: 1,
        },
      ]}
      accounts={[]}
      onRestoreControl={() => undefined}
      restorePending={false}
    />
  );
}

it.each([
  { selected: ["7"], expected: ["7", "6"], checked: true, action: "勾选" },
  { selected: ["7", "6"], expected: ["7"], checked: false, action: "取消勾选" },
])("$action 可见分组时保留已配置但列表暂未返回的稳定 ID", async (row) => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<ManagedGroups selected={row.selected} onChange={onChange} />);

  const checkbox = screen.getByRole("checkbox", { name: /选择分组 主力组/ });
  await user.click(checkbox);

  expect(checkbox).toHaveAttribute("aria-checked", String(row.checked));
  expect(onChange).toHaveBeenLastCalledWith(
    expect.objectContaining({
      advanced_policy: expect.objectContaining({
        scope: { managed_group_mode: "selected", managed_group_ids: row.expected },
      }),
    }),
  );
});
