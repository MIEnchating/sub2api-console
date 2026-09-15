import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIPriceComparison } from "../price-comparison";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it("批量比对名为__proto__的模型时正常显示匹配状态", () => {
  const model = { model: "__proto__", input_ratio: "1", completion_ratio: "4" };
  const snapshot: NewAPIRemoteSnapshot = {
    groups: [],
    models: [model],
    unset_models: [],
    references: [],
    tool_prices: [],
    differences: [],
    fetched_at: "2026-09-14T00:00:00Z",
    upstream_prices: [
      { host: "upstream.example", name: "上游", upstream_type: "sub2api", models: [model] },
    ],
  };
  render(<NewAPIPriceComparison snapshot={snapshot} />);
  fireEvent.click(screen.getByRole("combobox", { name: "比对上游" }));
  fireEvent.click(screen.getByRole("option", { name: "上游 · Sub2API" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择当前页全部模型" }));
  fireEvent.click(screen.getByRole("button", { name: "批量比对" }));
  expect(screen.getByLabelText("__proto__ 比对结果")).toHaveTextContent("一致");
});
