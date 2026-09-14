import { describe, expect, it } from "vitest";
import {
  dictionaryLabel,
  groupStatusDictionary,
  orderedDictionaryOptions,
  runtimeStatusDictionary,
  schedulingStrategyDictionary,
  taskStatusDictionary,
} from "../domain-dictionaries";

describe("领域字典", () => {
  it("使用调度策略别名返回统一展示名", () => {
    expect(schedulingStrategyDictionary.cost_first).toBe("价格优先");
    expect(schedulingStrategyDictionary.stability_first).toBe("稳定优先");
  });

  it("保留状态字典的展示色调", () => {
    expect(groupStatusDictionary.rate_limited).toEqual({ label: "限流中", tone: "warning" });
  });

  it("提供运行状态的统一展示名", () => {
    expect(runtimeStatusDictionary.running).toBe("运行中");
  });

  it("未知或空值使用稳定回退文案", () => {
    expect(dictionaryLabel(taskStatusDictionary, "missing")).toBe("配置错误");
    expect(dictionaryLabel(taskStatusDictionary, "")).toBe("配置错误");
  });

  it("按字典顺序返回启用项并追加未登记值", () => {
    expect(
      orderedDictionaryOptions(
        [
          { value: "openai", name: "OpenAI", enabled: true },
          { value: "anthropic", name: "Anthropic", enabled: false },
        ],
        [
          { value: "anthropic", label: "Anthropic" },
          { value: "custom", label: "Custom" },
        ],
      ),
    ).toEqual([
      { value: "openai", label: "OpenAI" },
      { value: "custom", label: "Custom" },
    ]);
  });
});
