import { beforeEach, describe, expect, it, vi } from "vitest";

const toastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: { error: toastError },
}));

import { createConsoleQueryClient } from "../query-client";

describe("global query feedback", () => {
  beforeEach(() => {
    toastError.mockClear();
  });

  it("sends every query failure to the floating message channel", async () => {
    const client = createConsoleQueryClient();

    await expect(
      client.fetchQuery({
        queryKey: ["failed-request"],
        queryFn: () => Promise.reject(new Error("请求失败 (502)")),
        retry: false,
      }),
    ).rejects.toThrow("请求失败 (502)");

    expect(toastError).toHaveBeenCalledWith("请求失败 (502)", {
      id: "operation-error:请求失败 (502)",
    });
    client.clear();
  });
});
