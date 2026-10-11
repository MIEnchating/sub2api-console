// @vitest-environment node
import { renderToStaticMarkup } from "react-dom/server";
import { bench } from "vitest";
import { GroupAllocationContent } from "../group-allocation-dialog";
import { allocationFixture } from "./allocation-fixture";

const allocation = allocationFixture(1000);

bench(
  "首次展示1000个账号的分组调度状态",
  () => {
    const markup = renderToStaticMarkup(<GroupAllocationContent allocation={allocation} />);
    if (!markup.includes("账号 6-1</span>") || !markup.includes("400 / 400"))
      throw new Error("账号或汇总丢失");
  },
  { time: 1500, warmupTime: 500 },
);
