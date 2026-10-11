import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "@/test/dictionary";
import { NewAPIGroupBindings } from "../group-bindings";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

const groups = [
  { id: "vip", name: "VIP", ratio: "1" },
  { id: "premium", name: "高级", ratio: "1" },
];
const localGroups = [{ id: "6", name: "标准", ratio: "0.5" }];
const binding = {
  platform_id: "primary",
  newapi_group_id: "vip",
  newapi_group_name: "VIP",
  sub2api_group_id: "6",
  sync_ratio: false,
};

it("只编辑同步开关后刷新倍率，保留开关草稿并采用服务器新倍率", () => {
  const save = vi.fn();
  const view = render(
    <NewAPIGroupBindings
      groups={[groups[0]]}
      localGroups={localGroups}
      bindings={[binding]}
      pending={false}
      onSave={save}
    />,
  );
  fireEvent.click(screen.getByRole("switch", { name: "VIP 倍率同步" }));
  view.rerender(
    <NewAPIGroupBindings
      groups={[groups[0]]}
      localGroups={[{ ...localGroups[0], ratio: "0.8" }]}
      bindings={[binding]}
      pending={false}
      onSave={save}
    />,
  );
  expect(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toHaveValue("0.8");
  expect(screen.getByRole("switch", { name: "VIP 倍率同步" })).toBeChecked();
});

it("编辑倍率后将另一行绑定到同一分组，采用当前草稿倍率并一致提交", async () => {
  const save = vi.fn();
  const user = userEvent.setup();
  render(
    <NewAPIGroupBindings
      groups={groups}
      localGroups={localGroups}
      bindings={[binding]}
      pending={false}
      onSave={save}
    />,
    { group: [{ value: "6" }] },
  );
  fireEvent.change(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" }), {
    target: { value: "0.75" },
  });
  await user.click(screen.getByRole("combobox", { name: "高级 的 Sub2API 分组" }));
  await user.click(await screen.findByRole("option", { name: "标准 · 0.5" }));
  expect(screen.getByRole("textbox", { name: "高级 的 Sub2API 管理平台倍率" })).toHaveValue("0.75");
  await user.click(screen.getByRole("button", { name: "保存绑定与倍率" }));
  expect(save).toHaveBeenCalledWith(
    groups.map((group) => ({
      newapi_group_id: group.id,
      newapi_group_name: group.name,
      sub2api_group_id: "6",
      sub2api_ratio: "0.75",
      sync_ratio: false,
    })),
  );
});

it("倍率草稿未保存时刷新新增同组绑定，新增行与已有行采用同一草稿倍率", () => {
  const save = vi.fn();
  const view = render(
    <NewAPIGroupBindings
      groups={[groups[0]]}
      localGroups={localGroups}
      bindings={[binding]}
      pending={false}
      onSave={save}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" }), {
    target: { value: "0.75" },
  });
  view.rerender(
    <NewAPIGroupBindings
      groups={groups}
      localGroups={[{ ...localGroups[0], ratio: "0.6" }]}
      bindings={[binding, { ...binding, newapi_group_id: "premium", newapi_group_name: "高级" }]}
      pending={false}
      onSave={save}
    />,
  );
  expect(screen.getByRole("textbox", { name: "高级 的 Sub2API 管理平台倍率" })).toHaveValue("0.75");
});
