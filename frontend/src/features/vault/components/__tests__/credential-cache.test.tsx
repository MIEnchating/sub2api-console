import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { VaultPage } from "../vault-page";

afterEach(() => vi.unstubAllGlobals());

it.each([true, false])(
  "密码箱提交成功=%s时密码只进入请求且不进入共享变更缓存",
  async (succeeded) => {
    const client = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
    });
    client.setQueryData(["auth-recovery-config"], { vault_entries: [], auth_records: [] });
    let requestBody = "";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        requestBody = String(init?.body);
        return new Response(
          JSON.stringify(succeeded ? { configured: true } : { error: "保存失败" }),
          {
            status: succeeded ? 200 : 503,
          },
        );
      }),
    );
    const view = render(
      <QueryClientProvider client={client}>
        <VaultPage />
      </QueryClientProvider>,
    );
    try {
      fireEvent.click(screen.getByRole("button", { name: "添加凭据" }));
      fireEvent.change(screen.getByLabelText("凭据名称"), { target: { value: "operator" } });
      fireEvent.change(screen.getByLabelText("密码"), {
        target: { value: "private-vault-password" },
      });
      fireEvent.click(screen.getAllByRole("button", { name: "添加凭据" }).at(-1)!);
      await waitFor(() => expect(requestBody).toContain("private-vault-password"));
      await waitFor(() => expect(client.isMutating()).toBe(0));
      expect(
        JSON.stringify(
          client
            .getMutationCache()
            .getAll()
            .map((mutation) => mutation.state),
        ),
      ).not.toContain("private-vault-password");
    } finally {
      view.unmount();
      client.clear();
    }
  },
);
