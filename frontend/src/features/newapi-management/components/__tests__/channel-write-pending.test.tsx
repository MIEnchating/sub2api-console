import { render } from "@/test/dictionary";
import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { NewAPIChannelForm } from "../channel-form";
import { NewAPIChannelConfigurationStep } from "../channel-configuration-step";

it("创建渠道密钥等待期间锁定账号来源和分组，失败后可以继续编辑", () => {
  const props = {
    groups: [{ id: "6", name: "标准", ratio: "1" }],
    newAPIGroups: [{ id: "default", name: "默认", ratio: "1" }],
    sub2APIBaseURL: "https://sub2api.example.test",
    vaultEntries: [],
    pending: false,
    creatingKey: false,
    fetchingModels: false,
    onCreateKey: vi.fn(),
    onFetchModels: vi.fn(),
    onSubmit: vi.fn(),
  };
  const view = render(<NewAPIChannelForm {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "自定义账号密码" }));
  fireEvent.change(screen.getByLabelText("登录邮箱"), { target: { value: "user@example.test" } });
  view.rerender(<NewAPIChannelForm {...props} creatingKey />);
  expect(screen.getByRole("button", { name: "密码箱账号" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "Sub2API 分组" })).toBeDisabled();
  expect(screen.getByLabelText("登录邮箱")).toBeDisabled();
  expect(screen.getByLabelText("密码")).toBeDisabled();
  view.rerender(<NewAPIChannelForm {...props} />);
  expect(screen.getByRole("combobox", { name: "Sub2API 分组" })).toBeEnabled();
  expect(screen.getByLabelText("登录邮箱")).toHaveValue("user@example.test");
});

it("添加渠道等待期间锁定分组与模型读取，失败后恢复操作", () => {
  const props = {
    channelName: "标准",
    sub2APIBaseURL: "https://sub2api.example.test",
    apiEndpoints: [],
    baseURL: "https://sub2api.example.test",
    customBaseURL: false,
    newAPIGroupOptions: [{ value: "default", label: "默认" }],
    selectedGroups: ["default"],
    selectedModelCount: 1,
    pending: true,
    fetchingModels: false,
    onFetchModels: vi.fn(),
    onBaseURLModeChange: vi.fn(),
    onBaseURLChange: vi.fn(),
    onGroupsChange: vi.fn(),
  };
  const view = render(<NewAPIChannelConfigurationStep {...props} />);
  expect(screen.getByRole("combobox", { name: "New API 分组" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: "从上游获取" })).toBeDisabled();
  view.rerender(<NewAPIChannelConfigurationStep {...props} pending={false} />);
  expect(screen.getByRole("combobox", { name: "New API 分组" })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: "从上游获取" })).toBeEnabled();
});
