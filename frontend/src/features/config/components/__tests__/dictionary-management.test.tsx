import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DictionaryEntry } from "@/api";
import { DictionaryManagement } from "../dictionary-management";

function entry(id: string, name: string, sortOrder: number): DictionaryEntry {
  return {
    id,
    name,
    kind: "platform",
    value: id,
    sort_order: sortOrder,
    enabled: true,
    description: "",
    created_at: "",
    updated_at: "",
    version: 1,
  };
}

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup(): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["dictionaries", "platform"], {
    items: [entry("openai", "OpenAI", 0), entry("gemini", "Gemini", 1)],
  });
  render(
    <QueryClientProvider client={client}>
      <DictionaryManagement />
    </QueryClientProvider>,
  );
  return client;
}

function rowNames(): string[] {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[1].textContent ?? "");
}

describe("字典排序响应", () => {
  it("排序操作同步显示新顺序，保存失败后恢复原顺序", async () => {
    let finish!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve;
          }),
      ),
    );
    setup();
    fireEvent.click(screen.getByRole("button", { name: "下移OpenAI" }));
    expect(rowNames()).toEqual(["Gemini", "OpenAI"]);
    await waitFor(() => expect(finish).toBeDefined());
    await act(async () => {
      finish(new Response(JSON.stringify({ detail: "保存失败" }), { status: 500 }));
    });
    await waitFor(() => expect(rowNames()).toEqual(["OpenAI", "Gemini"]));
    expect(screen.getByRole("button", { name: "下移OpenAI" })).toBeEnabled();
  });

  it("保存成功后无需重新同步字典即可继续排序", async () => {
    const fetcher = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal("fetch", fetcher);
    setup();
    fireEvent.click(screen.getByRole("button", { name: "下移OpenAI" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "下移Gemini" })).toBeEnabled());
    expect(rowNames()).toEqual(["Gemini", "OpenAI"]);
    expect(fetcher.mock.calls).toHaveLength(1);
  });

  it("保存中切换分类再返回仍显示待保存顺序，失败只回退原分类", async () => {
    let finish!: (response: Response) => void;
    const fetcher = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetcher);
    const client = setup();
    client.setQueryData(["dictionaries", "group"], {
      items: [{ ...entry("42", "主分组", 0), kind: "group" }],
    });
    fireEvent.click(screen.getByRole("button", { name: "下移OpenAI" }));
    await waitFor(() => expect(rowNames()).toEqual(["Gemini", "OpenAI"]));
    expect(screen.getByRole("button", { name: "刷新字典" })).toBeDisabled();
    fireEvent.click(screen.getByRole("tab", { name: "分组字典" }));
    expect(rowNames()).toEqual(["主分组"]);
    await act(async () => {
      finish(new Response(JSON.stringify({ detail: "排序冲突" }), { status: 409 }));
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "刷新字典" })).toBeEnabled());
    expect(rowNames()).toEqual(["主分组"]);
    fireEvent.click(screen.getByRole("tab", { name: "平台字典" }));
    expect(rowNames()).toEqual(["OpenAI", "Gemini"]);
    expect(fetcher.mock.calls).toHaveLength(1);
  });
});
