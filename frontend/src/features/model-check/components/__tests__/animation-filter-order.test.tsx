import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { account } from "@/features/accounts/__tests__/fixtures";
import { AnimationAccountFilters } from "../animation-account-filters";
import { defaultAnimationFilters } from "../../lib/animation-filters";

it("平台字典调整顺序后动画筛选使用后台顺序且保留新平台", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(["dictionaries", "platform"], {
    items: [
      { value: "openai", enabled: true },
      { value: "anthropic", enabled: true },
    ],
  });
  render(
    <QueryClientProvider client={client}>
      <AnimationAccountFilters
        accounts={[
          { ...account, id: "1", platform: "anthropic", groups: [] },
          { ...account, id: "2", platform: "openai", groups: [] },
          { ...account, id: "3", platform: "custom", groups: [] },
        ]}
        value={defaultAnimationFilters}
        onChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "平台筛选" }));
  const options = await screen.findAllByRole("option");
  expect(options.map((option) => option.textContent)).toEqual(["openai", "anthropic", "custom"]);
  client.clear();
});

it("分组字典逆序时动画筛选按字典名称展示并保留未登记分组", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["dictionaries", "group"], {
    items: [
      { value: "2", name: "B 分组", enabled: true },
      { value: "1", name: "A 分组", enabled: true },
    ],
  });
  render(
    <QueryClientProvider client={client}>
      <AnimationAccountFilters
        accounts={[{ ...account, groups: ["A 分组", "B 分组", "新分组"] }]}
        value={defaultAnimationFilters}
        onChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "分组筛选" }));
  expect((await screen.findAllByRole("option")).map((option) => option.textContent)).toEqual([
    "B 分组",
    "A 分组",
    "新分组",
  ]);
  client.clear();
});
