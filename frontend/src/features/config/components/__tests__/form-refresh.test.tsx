import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { RenderResult } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AccountCreationPolicyForm } from "../account-creation-policy-form";
import { PlatformProbeModelsForm } from "../platform-probe-models-form";

type FormProps = { model: string; onSubmit: () => Promise<void> };
const forms = [
  {
    name: "账号策略",
    field: "全局默认 账号模型",
    submit: "保存全局默认",
    render: (props: FormProps): ReactElement => (
      <AccountCreationPolicyForm
        formId="policy"
        scopeLabel="全局默认"
        policy={{
          models: [props.model],
          concurrency: 10,
          load_factor: null,
          priority: 1,
          pool_mode: false,
          pool_mode_retry_count: 3,
          pool_mode_retry_status_codes: [429],
        }}
        pending={false}
        submitLabel="保存全局默认"
        onSubmit={props.onSubmit}
      />
    ),
  },
  {
    name: "探活模型",
    field: "OpenAI 默认探活模型",
    submit: "保存默认探活模型",
    render: (props: FormProps): ReactElement => (
      <PlatformProbeModelsForm
        models={{ openai: props.model }}
        pending={false}
        onSubmit={props.onSubmit}
      />
    ),
  },
];

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

describe.each(forms)("$name 表单与服务端快照同步", (fixture) => {
  function setup(model: string, onSubmit: () => Promise<void>): RenderResult {
    const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    clients.push(client);
    return render(fixture.render({ model, onSubmit }), {
      wrapper: (props): ReactElement => (
        <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
      ),
    });
  }

  it("后台刷新后保存成功但新快照尚未到达时，保留提交值并清除未保存状态", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn(async (): Promise<void> => {});
    const view = setup("model-original", onSubmit);
    const field = screen.getByRole("textbox", { name: fixture.field });
    await user.clear(field);
    await user.type(field, "model-draft");
    view.rerender(fixture.render({ model: "model-refresh", onSubmit }));
    expect(field).toHaveValue("model-draft");

    await user.click(screen.getByRole("button", { name: fixture.submit }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: fixture.submit })).toBeDisabled(),
    );
    expect(field).toHaveValue("model-draft");
    expect(field).toBeEnabled();

    view.rerender(fixture.render({ model: "model-saved", onSubmit }));
    expect(field).toHaveValue("model-saved");
    expect(screen.getByRole("button", { name: fixture.submit })).toBeDisabled();
  });

  it("没有未保存编辑时，新服务端快照更新字段且保持保存按钮禁用", () => {
    const onSubmit = vi.fn(async (): Promise<void> => {});
    const view = setup("model-original", onSubmit);
    view.rerender(fixture.render({ model: "model-refresh", onSubmit }));
    expect(screen.getByRole("textbox", { name: fixture.field })).toHaveValue("model-refresh");
    expect(screen.getByRole("button", { name: fixture.submit })).toBeDisabled();
  });
});
