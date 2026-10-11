import { QueryClient, QueryClientProvider, onlineManager } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { ProfilePage } from "../profile-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function renderProfile(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["session"], { authenticated: true, username: "operator" });
  render(
    <QueryClientProvider client={client}>
      <ProfilePage />
    </QueryClientProvider>,
  );
}

function editProfile(): void {
  fireEvent.change(screen.getByLabelText("账号"), { target: { value: "renamed" } });
  fireEvent.change(screen.getByLabelText("当前密码"), { target: { value: "current-password" } });
}

it("编辑期间后台账号更新保留未提交资料和密码", async () => {
  renderProfile();
  editProfile();
  act(() => client.setQueryData(["session"], { authenticated: true, username: "external-name" }));
  await screen.findByText("external-name");
  expect(screen.getByLabelText("账号")).toHaveValue("renamed");
  expect(screen.getByLabelText("当前密码")).toHaveValue("current-password");
});

it.each(["before", "during"])("保存%s阶段启动的旧会话读取不会覆盖已保存账号", async (phase) => {
  let finishRead!: (response: Response) => void;
  let finishSave!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    (path: string) =>
      new Promise<Response>((resolve) => {
        if (path.endsWith("/profile")) finishSave = resolve;
        else finishRead = resolve;
      }),
  );
  renderProfile();
  editProfile();
  const read = (): Promise<unknown> =>
    client
      .fetchQuery({
        queryKey: ["session"],
        queryFn: ({ signal }) => api.session(signal),
        staleTime: 0,
      })
      .catch(() => null);
  let oldRead: Promise<unknown>;
  if (phase === "before") oldRead = read();
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(finishSave).toBeDefined());
  if (phase === "during") oldRead = read();
  await act(async () => finishSave(Response.json({ authenticated: true, username: "renamed" })));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled());
  await act(async () => {
    finishRead(Response.json({ authenticated: true, username: "operator" }));
    await oldRead;
  });
  expect(screen.getByLabelText("账号")).toHaveValue("renamed");
  expect(client.getQueryData(["session"])).toMatchObject({ username: "renamed" });
});

it("浏览器报告离线时提交仍尝试本地API，失败保留编辑并恢复入口", async () => {
  const fetch = vi.fn(async () => {
    throw new TypeError("无法连接本地 API");
  });
  vi.stubGlobal("fetch", fetch);
  renderProfile();
  onlineManager.setOnline(false);
  editProfile();
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled());
  expect(screen.getByLabelText("当前密码")).toHaveValue("current-password");
});

it("保存响应未确认认证时保留原会话和输入，不提示成功", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ authenticated: false, username: null }));
  renderProfile();
  editProfile();
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled());
  expect(screen.getByLabelText("当前密码")).toHaveValue("current-password");
  expect(client.getQueryData(["session"])).toMatchObject({
    authenticated: true,
    username: "operator",
  });
});
