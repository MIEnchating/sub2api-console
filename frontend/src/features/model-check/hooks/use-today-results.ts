import { useEffect, useMemo, useState } from "react";
import type { AnimationResult } from "@/api";

const dayMilliseconds = 86_400_000;
const beijingOffset = 8 * 3_600_000;

function beijingDay(timestamp: number): number {
  return Math.floor((timestamp + beijingOffset) / dayMilliseconds);
}

export function useTodayResults(
  results: Map<string, AnimationResult>,
  modelResults?: Map<string, AnimationResult[]>,
): { results: Map<string, AnimationResult>; modelResults: Map<string, AnimationResult[]> } {
  const [day, setDay] = useState(() => beijingDay(Date.now()));
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const update = (): void => {
      clearTimeout(timer);
      const now = Date.now();
      const current = beijingDay(now);
      setDay(current);
      timer = setTimeout(update, (current + 1) * dayMilliseconds - beijingOffset - now);
    };
    update();
    window.addEventListener("focus", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("focus", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);
  return useMemo(() => {
    const today = (result: AnimationResult): boolean =>
      beijingDay(Date.parse(result.completed_at)) === day;
    return {
      results: new Map([...results].filter(([, result]) => today(result))),
      modelResults: new Map(
        [...(modelResults ?? [])].map(([id, values]) => [id, values.filter(today)]),
      ),
    };
  }, [day, results, modelResults]);
}
