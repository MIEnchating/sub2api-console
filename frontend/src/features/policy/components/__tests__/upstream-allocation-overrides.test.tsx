import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  UpstreamAllocationOverrides,
  validAllocationOverrides,
} from "../upstream-allocation-overrides";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("策略页单独设置通过键盘关闭或恢复跟随时只修改指定稳定 ID", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const user = userEvent.setup();
  const change = vi.fn();
  render(
    <UpstreamAllocationOverrides
      label="账号单独设置"
      values={{ "147": true, "148": false }}
      names={new Map([["147", "账号147"]])}
      onChange={change}
    />,
  );
  const toggle = screen.getByRole("switch", { name: "账号147共享并发分配" });
  toggle.focus();
  await user.keyboard(" ");
  expect(change).toHaveBeenCalledWith({ "147": false, "148": false });
  await user.click(screen.getByRole("button", { name: "账号147恢复跟随" }));
  expect(change).toHaveBeenLastCalledWith({ "148": false });
});

it("非法单独设置不能通过前端策略校验，显式关闭保留", () => {
  expect(validAllocationOverrides({ "147": false }, true)).toBe(true);
  expect(validAllocationOverrides({ "147": "true" }, true)).toBe(false);
  expect(validAllocationOverrides({ accountName: true }, true)).toBe(false);
  expect(validAllocationOverrides(null, false)).toBe(false);
});
