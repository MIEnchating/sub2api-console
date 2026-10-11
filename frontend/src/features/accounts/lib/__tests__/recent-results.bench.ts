import { bench } from "vitest";
import type { AccountRecentResult } from "@/api";
import { mergeAccountResults } from "../recent-results";

const previous: AccountRecentResult[] = Array.from({ length: 100 }, (_, index) => ({
  id: String(100 - index),
  result: "通过",
  observed_at: "2026-10-09T08:00:00.000000000Z",
  latency_ms: 100,
  failure_reason: null,
  source: "traffic",
}));
const incoming = previous.map((result) => ({ ...result, latency_ms: 200 }));

bench("20 个账号各合并 100 条同秒请求更新", () => {
  for (let account = 0; account < 20; account++) {
    const results = mergeAccountResults(previous, incoming);
    if (results.length !== 100 || results[0].id !== "100" || results[0].latency_ms !== 200) {
      throw new Error("近期结果更新未保留最新请求");
    }
  }
});
