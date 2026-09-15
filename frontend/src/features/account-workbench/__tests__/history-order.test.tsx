import { fireEvent, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { WorkbenchHistory } from "../components/workbench-history";
import { workbenchKeys } from "../constants";

it("字典把失败置前时处理状态遵循配置且全部状态固定首位", async () => {
  const view = renderWithDictionaries(<></>, {
    task_status: [{ value: "failed" }, { value: "queued" }],
  });
  view.client.setQueryData(workbenchKeys.history, []);
  view.rerender(<WorkbenchHistory />);
  fireEvent.click(screen.getByRole("combobox", { name: "筛选处理状态" }));
  const options = await screen.findAllByRole("option");
  expect(options.slice(0, 3).map((option) => option.textContent)).toEqual([
    "全部状态",
    "处理失败",
    "等待执行",
  ]);
});
