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
