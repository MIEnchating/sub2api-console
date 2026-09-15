import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIPriceComparison } from "../price-comparison";

it("上游名称与地址相同时，选项及选中项展示一次地址并附带上游类型", async () => {
  const user = userEvent.setup();
  const snapshot: NewAPIRemoteSnapshot = {
    groups: [],
    models: [],
    unset_models: [],
    tool_prices: [],
    references: [],
    differences: [],
    fetched_at: "2026-09-14T00:00:00Z",
    upstream_prices: [
      {
        host: "api.example.test",
        name: "api.example.test",
        upstream_type: "sub2api",
        models: [],
      },
      {
        host: "192.0.2.1:3000",
        name: "192.0.2.1:3000",
        upstream_type: "newapi",
        models: [],
      },
    ],
  };
  render(<NewAPIPriceComparison snapshot={snapshot} />);

  const selector = screen.getByRole("combobox", { name: "比对上游" });
  await user.click(selector);

  expect(screen.getByRole("option", { name: "api.example.test · Sub2API" })).toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: "192.0.2.1:3000 · New API" }));

  expect(selector).toHaveTextContent(/^192\.0\.2\.1:3000 · New API$/);
  expect(selector).toHaveAttribute("aria-expanded", "false");
  await user.click(selector);
  expect(screen.getByRole("option", { name: "192.0.2.1:3000 · New API" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});
