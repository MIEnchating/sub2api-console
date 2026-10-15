import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { AnimationQualityBasis } from "../animation-quality-basis";

it("旧评审没有结构化依据时不补造证据或显示空区域", () => {
  render(<AnimationQualityBasis basis={undefined} />);
  expect(screen.queryByRole("region", { name: "自动判断依据" })).not.toBeInTheDocument();
});

it("多项依据默认收起，键盘展开后保留原文顺序和安全滚动区域", async () => {
  const source = `<script>window.untrusted = true;</script>\n<svg>${"x".repeat(1200)}</svg>`;
  render(
    <AnimationQualityBasis
      basis={[
        { requirement: "主体可辨认", evidence: source, assessment: "主体结构由 SVG 描述。" },
        { requirement: "车轮转动", evidence: "wheel.rotate()", assessment: "车轮有旋转调用。" },
      ]}
    />,
  );
  const region = screen.getByRole("region", { name: "自动判断依据" });
  const toggle = within(region).getByRole("button", { name: /原文依据/ });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(within(region).queryByRole("term")).not.toBeInTheDocument();
  toggle.focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  const terms = within(region).getAllByRole("term");
  expect(terms).toHaveLength(2);
  expect(terms[0]).toHaveTextContent("主体可辨认");
  expect(terms[1]).toHaveTextContent("车轮转动");
  const code = within(region).getByText(
    (_, element) => element?.tagName === "CODE" && element.textContent === source,
  );
  expect(code.textContent).toBe(source);
  expect(code.querySelector("script")).toBeNull();
  expect(code.parentElement).toHaveClass(
    "max-h-40",
    "overflow-auto",
    "whitespace-pre-wrap",
    "break-all",
  );
  await userEvent.setup().click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(toggle).toHaveFocus();
  expect(within(region).queryByRole("term")).not.toBeInTheDocument();
});
