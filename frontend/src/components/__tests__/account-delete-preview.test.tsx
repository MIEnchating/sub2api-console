import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AccountDeletePreviewDetails } from "../../App";
import type { AccountDeletePreview } from "../../api";

function preview(binding: AccountDeletePreview["binding"]): AccountDeletePreview {
  return {
    account_id: "174",
    account_name: "星筱AI-0.125",
    groups: [],
    management_base_url: "https://management.example.test",
    binding,
  };
}

describe("account deletion preview", () => {
  it("shows management-only cleanup without implying an upstream key deletion", () => {
    const markup = renderToStaticMarkup(<AccountDeletePreviewDetails preview={preview(null)} />);

    expect(markup).toContain("星筱AI-0.125");
    expect(markup).toContain("将删除管理平台账号、确认其不存在并清理 Console 本地记录");
    expect(markup).toContain("不会删除任何上游 Key");
    expect(markup).not.toContain("上游地址");
    expect(markup).not.toContain("稳定上游身份");
  });

  it("shows the exact stable key scope when a binding exists", () => {
    const markup = renderToStaticMarkup(
      <AccountDeletePreviewDetails
        preview={preview({
          id: 91,
          upstream_id: "upstream-1",
          upstream_host: "https://upstream.example.test",
          auth_host: "upstream.example.test",
          upstream_key_id: "key-8",
          upstream_key_name: "special-key",
        })}
      />,
    );

    expect(markup).toContain("该账号绑定的上游 Key");
    expect(markup).toContain("https://upstream.example.test");
    expect(markup).toContain("key-8");
  });
});
