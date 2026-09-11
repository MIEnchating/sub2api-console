import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { RemoteModelPricesTable } from "../model-prices";
it("远程价格目录首次读取时展示骨架屏，不把数据尚未返回显示为空目录", () => {
  render(<RemoteModelPricesTable prices={[]} pending error="" />);
  const loading = screen.getByRole("status", { name: "正在获取远程模型价格" });
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByText("远程价卡未返回模型价格")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});
