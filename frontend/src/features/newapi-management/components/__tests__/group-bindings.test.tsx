import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

import { NewAPIGroupBindings } from "../group-bindings";

const groups = [
  { id: "vip", name: "VIP", ratio: "1" },
  { id: "premium", name: "高级", ratio: "2" },
];
const localGroups = [{ id: "6", name: "高速", ratio: "0.5" }];
const bindings = groups.map((group) => ({
  platform_id: "platform-1",
  newapi_group_id: group.id,
  newapi_group_name: group.name,
  sub2api_group_id: "6",
  sync_ratio: false,
}));

beforeAll(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterAll(() => vi.unstubAllGlobals());

describe("New API 分组绑定倍率", () => {
  it("编辑 Sub2API 管理平台倍率并开启同步后提交两端同步标记", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(
      <NewAPIGroupBindings
        groups={[groups[0]!]}
        localGroups={localGroups}
        bindings={[bindings[0]!]}
        pending={false}
        onSave={onSave}
      />,
    );

    const ratio = screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" });
    await user.clear(ratio);
    await user.type(ratio, "0.75");
    await user.click(screen.getByRole("switch", { name: "VIP 倍率同步" }));
    await user.click(screen.getByRole("button", { name: "保存绑定与倍率" }));

    expect(onSave).toHaveBeenCalledWith([
      {
        newapi_group_id: "vip",
        newapi_group_name: "VIP",
        sub2api_group_id: "6",
        sub2api_ratio: "0.75",
        sync_ratio: true,
      },
    ]);
  });

  it("同一 Sub2API 分组被多处绑定时统一更新它的倍率", async () => {
    const user = userEvent.setup();
    render(
      <NewAPIGroupBindings
        groups={groups}
        localGroups={localGroups}
        bindings={bindings}
        pending={false}
        onSave={vi.fn()}
      />,
    );

    const vipRatio = screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" });
    await user.clear(vipRatio);
    await user.type(vipRatio, "0.8");

    expect(screen.getByRole("textbox", { name: "高级 的 Sub2API 管理平台倍率" })).toHaveValue(
      "0.8",
    );
  });

  it("非正数倍率标记为无效并禁止保存", async () => {
    const user = userEvent.setup();
    render(
      <NewAPIGroupBindings
        groups={[groups[0]!]}
        localGroups={localGroups}
        bindings={[bindings[0]!]}
        pending={false}
        onSave={vi.fn()}
      />,
    );

    const ratio = screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" });
    await user.clear(ratio);
    await user.type(ratio, "0");

    expect(ratio).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("button", { name: "保存绑定与倍率" })).toBeDisabled();
  });
});
