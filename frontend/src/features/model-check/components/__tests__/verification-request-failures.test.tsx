import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { VerificationEvidence } from "../verification-evidence";

afterEach(cleanup);

it("所有请求结束但有超时时显示失败数量并能直接定位失败项", async () => {
  render(
    <VerificationEvidence
      result={{
        total: 2,
        passed: 1,
        probes: [
          { id: "math", label: "多步计算", status: "passed", answer: "42" },
          {
            id: "public_kbf_crypto_params_0",
            label: "知识边界 · crypto_params · 1",
            status: "error",
            error: "请求超时",
          },
        ],
      }}
    />,
  );
  expect(screen.getByText("请求失败 1 项")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "查看失败项" }));
  expect(
    within(screen.getByRole("region", { name: "检测项内容" })).getByText("请求超时"),
  ).toBeVisible();
});
