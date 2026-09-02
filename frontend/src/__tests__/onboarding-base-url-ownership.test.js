import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("账号 Base URL 配置归属", () => {
  it("只在添加上游阶段显示输入框，不在分组添加账号阶段重复设置", () => {
    const source = readFileSync(join(import.meta.dirname, "../App.tsx"), "utf8");

    expect(source.match(/label="账号 Base URL"/g)).toHaveLength(1);
    expect(source.match(/register\("account_base_url"\)/g)).toHaveLength(1);
    expect(source).toContain(
      "base_url: existingBinding ? undefined : verifiedUpstream?.account_base_url",
    );
  });
});
