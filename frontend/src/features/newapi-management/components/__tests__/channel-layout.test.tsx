import { screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render } from "@/test/dictionary";
import { NewAPIChannelForm } from "../channel-form";
import { NewAPIChannelConfigurationStep } from "../channel-configuration-step";
import { NewAPIFormSkeleton } from "../newapi-form-skeleton";

function renderCredentials() {
  return render(
    <NewAPIChannelForm
      groups={[]}
      newAPIGroups={[]}
      sub2APIBaseURL="https://sub2api.example.test"
      vaultEntries={[]}
      pending={false}
      creatingKey={false}
      fetchingModels={false}
      onCreateKey={vi.fn()}
      onFetchModels={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
}

it("渠道表单沿用全宽工作区和共享标题栏，字段在宽容器分为等宽两列", () => {
  const view = renderCredentials();
  expect(view.container.querySelector('[data-slot="card"]')).toHaveClass("w-full");
  expect(view.container.querySelector('[data-slot="card"]')).not.toHaveClass(
    "max-w-2xl",
    "mx-auto",
  );
  expect(view.container.querySelector('[data-slot="card-header"]')).toContainElement(
    screen.getByRole("heading", { name: "添加 Sub2API 渠道" }),
  );
  expect(view.container.querySelector('[data-slot="card-action"]')).toContainElement(
    screen.getByRole("list", { name: "添加渠道步骤" }),
  );
  expect(view.container.querySelector("[data-channel-credentials-layout]")).toHaveClass(
    "grid-cols-1",
    "@3xl/channel:grid-cols-2",
  );
});

it("凭据或本地分组为空时显示下一步指引并禁止创建密钥", () => {
  renderCredentials();
  expect(screen.getByText("暂无可用的密码箱账号，可切换为自定义账号密码。")).toBeVisible();
  expect(screen.getByText("暂无可用分组，请先配置 Sub2API 分组后刷新。")).toBeVisible();
  expect(screen.getByRole("button", { name: "创建密钥" })).toBeDisabled();
});

it("步骤与提交区在窄屏允许换行，按钮保持共享尺寸", () => {
  const view = renderCredentials();
  expect(screen.getByRole("list", { name: "添加渠道步骤" })).toHaveClass("flex", "flex-wrap");
  expect(view.container.querySelector('[data-slot="channel-form-footer"]')).toHaveClass(
    "flex-wrap",
  );
  expect(screen.getByRole("button", { name: "创建密钥" })).not.toHaveClass("h-10", "h-12");
});

it("第二步遇到超长渠道名时完整换行，未选模型时展示明确提示", () => {
  const name = "超长渠道名称".repeat(20);
  render(
    <NewAPIChannelConfigurationStep
      channelName={name}
      sub2APIBaseURL="https://sub2api.example.test"
      apiEndpoints={[]}
      baseURL="https://api.example.test"
      customBaseURL={false}
      newAPIGroupOptions={[]}
      selectedGroups={[]}
      selectedModelCount={0}
      pending={false}
      fetchingModels={false}
      onFetchModels={vi.fn()}
      onBaseURLModeChange={vi.fn()}
      onBaseURLChange={vi.fn()}
      onGroupsChange={vi.fn()}
    />,
  );
  expect(screen.getByText(name)).toHaveClass("wrap-anywhere");
  expect(screen.getByText(name)).not.toHaveClass("truncate");
  expect(screen.getByText("尚未选择模型")).toBeVisible();
  expect(screen.getByRole("button", { name: "添加渠道" })).toBeDisabled();
});

it("渠道加载骨架与实际表单使用相同全宽响应式布局", () => {
  const view = render(<NewAPIFormSkeleton channel label="正在读取渠道配置" />);
  expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  expect(view.container.querySelector('[data-slot="card"]')).toHaveClass("w-full");
  expect(view.container.querySelector('[data-slot="card"]')).not.toHaveClass(
    "max-w-2xl",
    "mx-auto",
  );
  expect(view.container.querySelector("[data-channel-credentials-layout]")).toHaveClass(
    "grid-cols-1",
    "@3xl/channel:grid-cols-2",
  );
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});

it("无校验错误时不为每个字段占据空白错误行，也不保留空侧栏标题", () => {
  const view = renderCredentials();
  expect(view.container.querySelector('[data-slot="field-error"]')).toBeNull();
  expect(screen.queryByRole("heading", { name: "渠道归属" })).not.toBeInTheDocument();
});
