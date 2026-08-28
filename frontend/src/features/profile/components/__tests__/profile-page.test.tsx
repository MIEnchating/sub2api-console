import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { SessionStatus } from "@/api";
import { ProfilePage } from "../profile-page";

describe("ProfilePage", () => {
  it("shows the current account and both protected update forms", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData<SessionStatus>(["session"], {
      authenticated: true,
      username: "operator",
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <ProfilePage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("operator");
    expect(markup).toContain("账号与密码");
    expect(markup).toContain("保存修改");
    expect(markup.match(/autoComplete="current-password"/g)).toHaveLength(1);
    expect(markup).toContain('autoComplete="new-password"');
  });
});
