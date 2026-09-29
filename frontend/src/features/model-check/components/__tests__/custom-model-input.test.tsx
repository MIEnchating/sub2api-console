import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useForm } from "react-hook-form";
import { expect, it } from "vitest";
import type { ReactElement } from "react";
import type { CustomAnimationForm } from "../../lib/animation-schema";
import { CustomAnimationModelField } from "../custom-animation-model-field";

function Settings(): ReactElement {
  const form = useForm<CustomAnimationForm>({
    defaultValues: {
      model: "gpt-6-astra",
      base_url: "",
      api_key: "",
      platform: "openai",
      timeout_seconds: 120,
    },
  });
  return <CustomAnimationModelField form={form} disabled={false} />;
}

it("手动输入模型后按 Escape 关闭建议时保留输入值", async () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
  try {
    const user = userEvent.setup();
    const model = screen.getByRole("combobox", { name: "检测模型" });
    expect(model).toHaveValue("gpt-6-astra");
    await user.clear(model);
    await user.type(model, "manual-model");
    await user.keyboard("{Escape}");
    expect(model).toHaveValue("manual-model");
  } finally {
    view.unmount();
    client.clear();
  }
});
