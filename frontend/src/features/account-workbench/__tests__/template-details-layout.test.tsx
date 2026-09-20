import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { TemplateDetails } from "../components/template-details";
import { template } from "./template-fixture";

afterEach(cleanup);

const mappedTemplate = {
  ...template,
  config: {
    ...template.config,
    credential_extras: {
      model_mapping: { alpha: "alpha", beta: "beta", gamma: "gamma", delta: "delta" },
    },
  },
};

it("多条模型映射独立占满详情宽度，不挤入基础参数列", () => {
  render(<TemplateDetails template={mappedTemplate} />);
  const mapping = screen.getByRole("table", { name: "模型映射" });
  expect(mapping).toHaveClass("w-full", "table-fixed");
  expect(within(mapping).getAllByRole("row")).toHaveLength(5);
  expect(screen.getByRole("region", { name: "模型映射" })).toHaveClass("col-span-full");
});

it("列表中多条映射先展示三条，键盘可展开和收起且状态同步", async () => {
  const user = userEvent.setup();
  render(<TemplateDetails template={mappedTemplate} compact />);
  const mapping = screen.getByRole("table", { name: "模型映射" });
  const toggle = screen.getByRole("button", { name: "展开全部 4 条映射" });
  expect(within(mapping).getAllByRole("row")).toHaveLength(4);
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(within(mapping).getAllByRole("row")).toHaveLength(5);
  await user.keyboard(" ");
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(within(mapping).getAllByRole("row")).toHaveLength(4);
});

it("单条长模型映射保留完整值并允许换行，不显示展开按钮", () => {
  const name = "long-model-name-".repeat(20);
  render(
    <TemplateDetails
      compact
      template={{
        ...mappedTemplate,
        config: {
          ...mappedTemplate.config,
          credential_extras: { model_mapping: { [name]: "target" } },
        },
      }}
    />,
  );
  expect(screen.getByRole("cell", { name })).toHaveClass("wrap-anywhere");
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
});

it("没有模型映射和分组时显示不限制与未分组", () => {
  render(
    <TemplateDetails
      template={{
        ...template,
        config: { ...template.config, credential_extras: {} },
        summary: { ...template.summary, groups: [] },
      }}
    />,
  );
  expect(screen.getByText("不限制")).toBeVisible();
  expect(screen.getByText("未分组")).toBeVisible();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});
