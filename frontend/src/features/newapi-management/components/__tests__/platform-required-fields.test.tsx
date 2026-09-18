import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { NewAPIPlatformDialog } from "../platform-dialog";

it("首次配置标明必填并阻止遗漏 Admin Key，原地址编辑允许留空保留", async () => {
  const submit = vi.fn();
  const view = render(
    <NewAPIPlatformDialog
      open
      platform={null}
      pending={false}
      onOpenChange={vi.fn()}
      onSubmit={submit}
    />,
  );
  const user = userEvent.setup();
  for (const label of ["平台名称", "平台地址", "User ID", "Admin Key"])
    expect(screen.getByLabelText(label)).toHaveAttribute("aria-required", "true");
  await user.type(screen.getByLabelText("平台名称"), "测试平台");
  await user.type(screen.getByLabelText("平台地址"), "https://new.example.test");
  await user.type(screen.getByLabelText("User ID"), "1");
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Admin Key");
  expect(submit).not.toHaveBeenCalled();
  view.rerender(
    <NewAPIPlatformDialog
      open
      platform={{
        id: "primary",
        name: "测试平台",
        base_url: "https://new.example.test",
        user_id: "1",
        admin_key_configured: true,
        updated_at: "",
      }}
      pending={false}
      onOpenChange={vi.fn()}
      onSubmit={submit}
    />,
  );
  expect(screen.getByLabelText("Admin Key")).toHaveAttribute("aria-required", "false");
  expect(screen.getByLabelText("Admin Key")).toHaveAccessibleDescription(/留空保留/);
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ admin_key: "" })),
  );
  await user.clear(screen.getByLabelText("平台地址"));
  await user.type(screen.getByLabelText("平台地址"), "https://other.example.test");
  expect(screen.getByLabelText("Admin Key")).toHaveAttribute("aria-required", "true");
});
