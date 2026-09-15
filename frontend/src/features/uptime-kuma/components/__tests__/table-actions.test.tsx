import { render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { MonitorWorkspace } from "../monitor-workspace";
import { monitor } from "./fixtures";

it("监控切换为仅查看模式时移除固定操作列，最后的在线率保留普通列", () => {
  const onAction = vi.fn();
  const onEdit = vi.fn();
  const view = render(
    <MonitorWorkspace
      monitors={[monitor]}
      management
      disabled={false}
      onAction={onAction}
      onEdit={onEdit}
    />,
  );
  const table = screen.getByRole("table");
  expect(table).toHaveAttribute("data-action-column", "true");
  expect(within(table).getByRole("columnheader", { name: "操作" })).toBeVisible();

  view.rerender(
    <MonitorWorkspace
      monitors={[monitor]}
      management={false}
      disabled={false}
      onAction={onAction}
      onEdit={onEdit}
    />,
  );

  expect(table).not.toHaveAttribute("data-action-column");
  expect(within(table).queryByRole("columnheader", { name: "操作" })).not.toBeInTheDocument();
  expect(within(table).getAllByRole("columnheader").at(-1)).toHaveTextContent("24 小时在线率");
});
