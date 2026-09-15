import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { WorkbenchMixedForm } from "../components/workbench-mixed-form";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
});

it("托管模板查询失败的缓存不会阻止独立本地混合转换", async () => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await client
    .fetchQuery({
      queryKey: workbenchKeys.templates,
      queryFn: () => Promise.reject(new Error("托管配置不可用")),
    })
    .catch(() => undefined);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMixedForm scope="local-export" onSubmit={() => {}} />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "账号内容" }), {
    target: { value: "rt_fixture" },
  });
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
});
