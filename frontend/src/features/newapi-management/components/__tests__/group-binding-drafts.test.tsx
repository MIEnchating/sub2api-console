import { render } from "@/test/dictionary";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { NewAPIGroupBindings } from "../group-bindings";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

const props = {
  groups: [{ id: "vip", name: "VIP", ratio: "1" }],
  localGroups: [
    { id: "6", name: "标准", ratio: "0.5" },
    { id: "7", name: "高速", ratio: "1" },
  ],
  bindings: [
    {
      platform_id: "primary",
      newapi_group_id: "vip",
      newapi_group_name: "VIP",
      sub2api_group_id: "6",
      sync_ratio: false,
    },
  ],
  pending: false,
};

it("后台分组刷新时保留未保存倍率，同时纳入新分组已有绑定", async () => {
  const save = vi.fn();
  const view = render(<NewAPIGroupBindings {...props} onSave={save} />);
  fireEvent.change(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" }), {
    target: { value: "0.75" },
  });
  view.rerender(
    <NewAPIGroupBindings
      {...props}
      groups={[...props.groups, { id: "premium", name: "高级", ratio: "1" }]}
      localGroups={[{ ...props.localGroups[0], ratio: "0.6" }, props.localGroups[1]]}
      bindings={[
        ...props.bindings,
        {
          platform_id: "primary",
          newapi_group_id: "premium",
          newapi_group_name: "高级",
          sub2api_group_id: "7",
          sync_ratio: true,
        },
      ]}
      onSave={save}
    />,
  );
  expect(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toHaveValue("0.75");
  fireEvent.click(screen.getByRole("button", { name: "保存绑定与倍率" }));
  expect(save).toHaveBeenCalledWith([
    {
      newapi_group_id: "vip",
      newapi_group_name: "VIP",
      sub2api_group_id: "6",
      sub2api_ratio: "0.75",
      sync_ratio: false,
    },
    {
      newapi_group_id: "premium",
      newapi_group_name: "高级",
      sub2api_group_id: "7",
      sub2api_ratio: "1",
      sync_ratio: true,
    },
  ]);
});

it("服务器读回已保存草稿后，后续外部倍率更新可以继续刷新", async () => {
  const view = render(<NewAPIGroupBindings {...props} onSave={vi.fn()} />);
  fireEvent.change(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" }), {
    target: { value: "0.75" },
  });
  view.rerender(
    <NewAPIGroupBindings
      {...props}
      localGroups={[{ ...props.localGroups[0], ratio: "0.75" }]}
      onSave={vi.fn()}
    />,
  );
  view.rerender(
    <NewAPIGroupBindings
      {...props}
      localGroups={[{ ...props.localGroups[0], ratio: "0.8" }]}
      onSave={vi.fn()}
    />,
  );
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toHaveValue("0.8"),
  );
});

it("保存绑定等待期间锁定分组、倍率和所有同步开关", () => {
  render(<NewAPIGroupBindings {...props} pending onSave={vi.fn()} />);
  expect(screen.getByRole("combobox", { name: "VIP 的 Sub2API 分组" })).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toBeDisabled();
  for (const control of screen.getAllByRole("switch"))
    expect(control).toHaveAttribute("aria-disabled", "true");
});

it("远端分组ID为__proto__时正确保存倍率与统一同步设置", () => {
  const save = vi.fn();
  render(
    <NewAPIGroupBindings
      {...props}
      groups={[{ id: "__proto__", name: "特殊分组", ratio: "1" }]}
      bindings={[
        { ...props.bindings[0], newapi_group_id: "__proto__", newapi_group_name: "特殊分组" },
      ]}
      onSave={save}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "特殊分组 的 Sub2API 管理平台倍率" }), {
    target: { value: "0.75" },
  });
  fireEvent.click(screen.getByRole("switch", { name: "统一倍率同步" }));
  fireEvent.click(screen.getByRole("button", { name: "保存绑定与倍率" }));
  expect(save).toHaveBeenCalledWith([
    {
      newapi_group_id: "__proto__",
      newapi_group_name: "特殊分组",
      sub2api_group_id: "6",
      sub2api_ratio: "0.75",
      sync_ratio: true,
    },
  ]);
});
