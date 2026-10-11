import { afterEach, expect, it, vi } from "vitest";
import { task } from "../../__tests__/fixtures";
import { waitForAccountSettingTasks } from "../account-settings-panel";

afterEach(() => vi.useRealTimers());
it("退出编辑时清除轮询等待，不再读取下一轮任务", async () => {
  vi.useFakeTimers();
  const controller = new AbortController();
  const load = vi.fn(async () => task("queued", "queued"));
  const result = waitForAccountSettingTasks(
    [task("queued", "queued")],
    load,
    undefined,
    controller.signal,
  );
  const rejected = expect(result).rejects.toMatchObject({ name: "AbortError" });
  await vi.advanceTimersByTimeAsync(0);
  expect(load).toHaveBeenCalledOnce();
  controller.abort();
  await rejected;
  expect(vi.getTimerCount()).toBe(0);
  await vi.advanceTimersByTimeAsync(1000);
  expect(load).toHaveBeenCalledOnce();
});
