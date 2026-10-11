import { bench } from "vitest";
import { formatDate } from "../date-format";

const timestamps = Array.from({ length: 5000 }, (_, index) =>
  new Date(1791504000000 + index * 1000).toISOString(),
);

bench("格式化 5000 条列表时间（分钟与秒交替）", () => {
  let characters = 0;
  for (let index = 0; index < timestamps.length; index++) {
    characters += formatDate(timestamps[index], index % 2 === 0).length;
  }
  if (characters !== 62_500) throw new Error("日期格式发生变化");
});
