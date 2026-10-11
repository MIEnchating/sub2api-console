import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIPriceComparison } from "../price-comparison";

function snapshot(): NewAPIRemoteSnapshot {
  return {
    groups: [],
    models: [{ model: "model-a", input_ratio: "1", completion_ratio: "2" }],
    unset_models: [],
    tool_prices: [],
    references: [],
    upstream_prices: [
      {
        host: "upstream.example.test",
        name: "上游 A",
        upstream_type: "sub2api",
        models: [{ model: "model-a", input_ratio: "1", completion_ratio: "2" }],
      },
    ],
    differences: [],
    fetched_at: "2026-10-10T00:00:00Z",
  };
}

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it.each(["platform", "upstream"])(
  "%s 价格刷新后清除旧比对结果，保留选择并按新价格重新比对",
  async (source) => {
    const user = userEvent.setup();
    const initial = snapshot();
    const view = render(<NewAPIPriceComparison snapshot={initial} />);
    await user.click(screen.getByRole("combobox", { name: "比对上游" }));
    await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
    await user.click(screen.getByRole("checkbox", { name: "选择 model-a" }));
    await user.click(screen.getByRole("button", { name: "批量比对" }));
    expect(screen.getByLabelText("model-a 比对结果")).toHaveTextContent("一致");

    const updated = snapshot();
    if (source === "platform") updated.models[0]!.input_ratio = "3";
    else updated.upstream_prices![0]!.models[0]!.input_ratio = "3";
    view.rerender(<NewAPIPriceComparison snapshot={updated} />);

    expect(screen.getByLabelText("model-a 比对结果")).toHaveTextContent("未比对");
    expect(screen.getByRole("checkbox", { name: "选择 model-a" })).toBeChecked();
    expect(screen.getByRole("combobox", { name: "比对上游" })).toHaveTextContent("上游 A");
    await user.click(screen.getByRole("button", { name: "批量比对" }));
    expect(screen.getByLabelText("model-a 比对结果")).toHaveTextContent("不一致");
  },
);

it("打开比对详情后价格刷新会关闭旧详情，重开使用当前价格", async () => {
  const user = userEvent.setup();
  const view = render(<NewAPIPriceComparison snapshot={snapshot()} />);
  await user.click(screen.getByRole("combobox", { name: "比对上游" }));
  await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
  await user.click(screen.getByRole("button", { name: "比对 model-a" }));
  expect(screen.getByText("价格一致")).toBeInTheDocument();

  const updated = snapshot();
  updated.models[0]!.input_ratio = "3";
  view.rerender(<NewAPIPriceComparison snapshot={updated} />);

  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "比对 model-a" }));
  expect(screen.getByText("价格不一致")).toBeInTheDocument();
});

it("刷新删除已选模型后清除失效选择，新增模型不会被自动选中", async () => {
  const user = userEvent.setup();
  const view = render(<NewAPIPriceComparison snapshot={snapshot()} />);
  await user.click(screen.getByRole("combobox", { name: "比对上游" }));
  await user.click(screen.getByRole("option", { name: "上游 A · Sub2API" }));
  await user.click(screen.getByRole("checkbox", { name: "选择 model-a" }));
  const updated = snapshot();
  updated.models = [{ model: "model-b", input_ratio: "1", completion_ratio: "2" }];
  view.rerender(<NewAPIPriceComparison snapshot={updated} />);

  expect(screen.getByRole("button", { name: "批量比对" })).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "选择 model-b" })).not.toBeChecked();
});
