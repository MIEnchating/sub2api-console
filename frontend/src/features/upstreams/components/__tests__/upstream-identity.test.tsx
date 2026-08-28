import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamIdentity, upstreamIdentityLayout } from "../upstream-identity";

describe("upstream identity", () => {
  it("shows the name and host together while keeping the host as an external link", () => {
    const markup = renderToStaticMarkup(
      <UpstreamIdentity
        name="生产环境上游"
        host="api.example.test"
        baseUrl="https://api.example.test/v1"
      />,
    );

    expect(markup).toContain("生产环境上游");
    expect(markup).toContain("api.example.test");
    expect(markup).toContain('href="https://api.example.test/v1"');
    expect(markup).toContain('target="_blank"');
    expect(markup).toContain('rel="noreferrer"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).not.toContain("title=");
  });

  it("keeps long names and hosts on stable truncated lines", () => {
    expect(upstreamIdentityLayout.root).toContain("min-w-0");
    expect(upstreamIdentityLayout.name).toContain("truncate");
    expect(upstreamIdentityLayout.link).toContain("min-w-0");
    expect(upstreamIdentityLayout.host).toContain("truncate");
  });

  it("uses an HTTPS host URL when the upstream has no base URL", () => {
    const markup = renderToStaticMarkup(
      <UpstreamIdentity name="备用上游" host="backup.example.test" baseUrl="" />,
    );

    expect(markup).toContain('href="https://backup.example.test"');
  });
});
