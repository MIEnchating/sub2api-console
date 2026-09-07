import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { PolicyOperationsEditor, PolicyRulesEditor } from "../../../App";

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
      breaker: { instant_status_codes: [] },
      cleanup: { trigger_status_codes: [] },
      probe: { retry_enabled: true, retry_source: "fixed", retry_status_codes: [] },
      classify: { fatal_patterns: [], gateway_status_codes: [], client_error_status_codes: [] },
    },
  };
}

function ListEditor(props: {
  section: "health" | "sampling" | "rules";
  onChange: (value: Draft) => void;
}) {
  const [draft, setDraft] = useState(initialDraft);
  function change(value: Draft): void {
    setDraft(value);
    props.onChange(value);
  }
  if (props.section === "rules") {
    return <PolicyRulesEditor value={draft} onChange={change} />;
  }
  return (
    <PolicyOperationsEditor
      section={props.section}
      value={draft}
      onChange={change}
      probesEnabled
      probesPending={false}
      onProbesEnabledChange={() => undefined}
    />
  );
}

describe("策略列表逐字输入", () => {
  it.each([
    ["health", "见到即熔断的错误码", "breaker", "instant_status_codes", "401, 403", [401, 403]],
    ["health", "触发处置的错误码", "cleanup", "trigger_status_codes", "401, 403", [401, 403]],
    ["sampling", "触发重试状态码", "probe", "retry_status_codes", "429, 503", [429, 503]],
    ["rules", "网关错误状态码", "classify", "gateway_status_codes", "500, 503", [500, 503]],
    ["rules", "客户端错误状态码", "classify", "client_error_status_codes", "400, 404", [400, 404]],
  ] as const)(
    "%s 中逐字输入 %s 时保留分隔符并提交两个状态码",
    async (section, label, group, field, text, expected) => {
      const user = userEvent.setup();
      const onChange = vi.fn();
      render(<ListEditor section={section} onChange={onChange} />);

      const input = screen.getByRole("textbox", { name: label });
      await user.type(input, text);

      expect(input).toHaveValue(text);
      expect(onChange).toHaveBeenLastCalledWith(
        expect.objectContaining({
          advanced_policy: expect.objectContaining({
            [group]: expect.objectContaining({ [field]: expected }),
          }),
        }),
      );
    },
  );

  it("逐字输入带空格的两行致命关键字时保留空格与换行", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ListEditor section="rules" onChange={onChange} />);

    const input = screen.getByRole("textbox", { name: "致命错误关键字（每行一个）" });
    await user.type(input, "api key invalid{Enter}token expired");

    expect(input).toHaveValue("api key invalid\ntoken expired");
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        advanced_policy: expect.objectContaining({
          classify: expect.objectContaining({
            fatal_patterns: ["api key invalid", "token expired"],
          }),
        }),
      }),
    );
  });

  it("清空已输入的状态码时提交空数组", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ListEditor section="rules" onChange={onChange} />);

    const input = screen.getByRole("textbox", { name: "网关错误状态码" });
    await user.type(input, "500, 503");
    await user.clear(input);

    expect(input).toHaveValue("");
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        advanced_policy: expect.objectContaining({
          classify: expect.objectContaining({ gateway_status_codes: [] }),
        }),
      }),
    );
  });

  it("父级替换策略值后显示新状态码并移除旧的编辑文本", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const view = render(<PolicyRulesEditor value={initialDraft()} onChange={onChange} />);
    const input = screen.getByRole("textbox", { name: "网关错误状态码" });
    await user.type(input, "500, ");

    const updated = initialDraft();
    updated.advanced_policy.classify = { gateway_status_codes: [502, 504] };
    view.rerender(<PolicyRulesEditor value={updated} onChange={onChange} />);

    expect(input).toHaveValue("502, 504");
  });
});
