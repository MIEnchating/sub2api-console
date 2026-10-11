// @vitest-environment node
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { bench } from "vitest";

import { LogStructuredValue } from "../log-details-dialog";

const value = {
  routing: {
    account_targets: Object.fromEntries(
      Array.from({ length: 5000 }, (_, index) => [
        String(index + 1),
        {
          account_name: `账号 ${index + 1}`,
          desired_concurrency: 5,
          desired_load_factor: "50",
          reason: "按当前健康状态调度",
        },
      ]),
    ),
  },
};

bench(
  "首次渲染折叠的 5000 个调度目标",
  () => {
    const markup = renderToStaticMarkup(createElement(LogStructuredValue, { value }));
    if (!markup.includes("5000 项") || !markup.includes("调度目标"))
      throw new Error("集合摘要丢失");
  },
  { time: 1500, warmupTime: 500 },
);

bench(
  "首次展示 5000 个调度目标中的第一页",
  () => {
    const markup = renderToStaticMarkup(
      createElement(LogStructuredValue, {
        value: value.routing.account_targets,
        fieldKey: "account_targets",
      }),
    );
    if (!markup.includes("账号 10</strong>") || markup.includes("账号 11</strong>"))
      throw new Error("分页明细不正确");
  },
  { time: 1500, warmupTime: 500 },
);
