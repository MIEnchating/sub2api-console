import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { ProfilePage } from "../profile-page";

afterEach(() => vi.unstubAllGlobals());

function renderProfile(authenticated = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(["session"], { authenticated, username: "operator" });
  const view = render(
    <QueryClientProvider client={client}>
      <ProfilePage />
    </QueryClientProvider>,
  );
  return { client, view };
}

function fillPasswords() {
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "current-private-password" },
  });
  fireEvent.change(screen.getByLabelText("新密码（可选）"), {
    target: { value: "new-private-password" },
  });
  fireEvent.change(screen.getByLabelText("确认新密码"), {
    target: { value: "new-private-password" },
  });
}

it.each([true, false])("个人信息提交成功=%s时密码不进入共享变更缓存", async (success) => {
  let body = "";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      body = String(init?.body);
      return new Response(
        JSON.stringify(
          success ? { authenticated: true, username: "operator" } : { error: "保存失败" },
        ),
        { status: success ? 200 : 503 },
      );
    }),
  );
  const { client, view } = renderProfile();
  try {
    fillPasswords();
    fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
    await waitFor(() => expect(body).toContain("current-private-password"));
    await waitFor(() => expect(client.isMutating()).toBe(0));
    const cached = JSON.stringify(
      client
        .getMutationCache()
        .getAll()
        .map((mutation) => mutation.state),
    );
    expect(cached).not.toContain("current-private-password");
    expect(cached).not.toContain("new-private-password");
    if (success) expect(screen.getByLabelText("当前密码")).toHaveValue("");
    else expect(screen.getByLabelText("当前密码")).toHaveValue("current-private-password");
  } finally {
    view.unmount();
    client.clear();
  }
});

it("个人信息保存期间冻结输入并在成功后清空密码", async () => {
  let resolve!: (response: Response) => void;
  const fetchMock = vi.fn(
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const { client, view } = renderProfile();
  try {
    fillPasswords();
    fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    for (const label of ["账号", "当前密码", "新密码（可选）", "确认新密码"]) {
      expect(screen.getByLabelText(label)).toBeDisabled();
    }
    resolve(new Response(JSON.stringify({ authenticated: true, username: "operator" })));
    await waitFor(() => expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled());
    expect(screen.getByLabelText("当前密码")).toHaveValue("");
  } finally {
    resolve?.(new Response("{}"));
    view.unmount();
    client.clear();
  }
});

it("会话未认证时禁用个人信息提交", () => {
  const { client, view } = renderProfile(false);
  try {
    expect(screen.getByRole("button", { name: "保存修改" })).toBeDisabled();
  } finally {
    view.unmount();
    client.clear();
  }
});

it("会话首次读取失败时禁用保存并允许重新读取", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ error: "读取失败" }), { status: 503 }))
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ authenticated: true, username: "operator" })),
    );
  vi.stubGlobal("fetch", fetchMock);
  const view = render(
    <QueryClientProvider client={client}>
      <ProfilePage />
    </QueryClientProvider>,
  );
  try {
    await waitFor(() => expect(client.getQueryState(["session"])?.status).toBe("error"));
    expect(screen.getByRole("button", { name: "保存修改" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
    await waitFor(() => expect(screen.getByLabelText("账号")).toHaveValue("operator"));
    expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled();
  } finally {
    view.unmount();
    client.clear();
  }
});
