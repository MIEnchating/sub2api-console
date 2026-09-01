import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamGroupBindingEditor } from "../upstream-group-binding-editor";

describe("UpstreamGroupBindingEditor", () => {
  it("renders only the local-group selector without a change summary", () => {
    const markup = renderToStaticMarkup(
      <UpstreamGroupBindingEditor
        upstreamGroupName="pro"
        groups={[
          { id: "6", name: "标准", rate_multiplier: "0.2" },
          { id: "7", name: "低价" },
          { id: "8", name: "新组" },
        ]}
        value={["7", "8"]}
        disabled={false}
        disabledReason={null}
        onValueChange={() => undefined}
      />,
    );

    expect(markup).not.toContain("新增：");
    expect(markup).not.toContain("移除：");
    expect(markup).toContain('aria-label="pro 本地分组"');
    expect(markup).not.toContain(">本地分组<");
  });
});
