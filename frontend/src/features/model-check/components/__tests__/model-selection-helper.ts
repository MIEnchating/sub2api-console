import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/** Select model IDs through the same search and multi-selection flow as a user. */
export async function selectDetectionModels(models: string[], replace = true): Promise<void> {
  const user = userEvent.setup();
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  if (trigger.getAttribute("aria-expanded") !== "true") await user.click(trigger);
  if (replace) {
    const clear = screen.getByRole("button", { name: "清空模型" });
    if (!clear.hasAttribute("disabled")) await user.click(clear);
  }
  for (const model of models) {
    const search = screen.getByRole("combobox", { name: "搜索或输入模型 ID" });
    await user.clear(search);
    await user.type(search, model);
    const option =
      screen.queryByRole("option", { name: model }) ??
      screen.getByRole("option", { name: `使用模型 ${model}` });
    if (option.getAttribute("aria-selected") !== "true") await user.click(option);
  }
  await user.keyboard("{Escape}");
}
