import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { isElementOverflowing } from "../ui/table-overflow-tooltip";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../ui/table";

describe("TableHeader", () => {
  it("uses the shared New API table header background token", () => {
    const markup = renderToStaticMarkup(
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Host</TableHead>
          </TableRow>
        </TableHeader>
      </Table>,
    );

    expect(markup).toContain("[background-color:var(--table-header)]");
    expect(markup).toContain("[&amp;_th]:sticky");
    expect(markup).toContain("[&amp;_th]:top-0");
  });

  it("enables single-line overflow tooltips by default without a native title", () => {
    const markup = renderToStaticMarkup(
      <Table>
        <TableBody>
          <TableRow>
            <TableCell title="完整内容">很长的内容</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(markup).toContain('data-overflow-tooltip="true"');
    expect(markup).toContain("table-fixed");
    expect(markup).toContain("truncate");
    expect(markup).not.toContain('title="完整内容"');
  });

  it("restores ordinary cell rendering when overflow tooltips are disabled", () => {
    const markup = renderToStaticMarkup(
      <Table overflowTooltip={false}>
        <TableBody>
          <TableRow>
            <TableCell>允许按页面布局显示</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(markup).toContain('data-overflow-tooltip="false"');
    expect(markup).not.toContain("table-fixed");
    expect(markup).not.toContain("truncate");
  });

  it("opens overflow content only when its rendered box is clipped", () => {
    expect(
      isElementOverflowing({
        clientHeight: 20,
        clientWidth: 100,
        scrollHeight: 20,
        scrollWidth: 101,
      }),
    ).toBe(true);
    expect(
      isElementOverflowing({
        clientHeight: 20,
        clientWidth: 100,
        scrollHeight: 20,
        scrollWidth: 100,
      }),
    ).toBe(false);
  });
});
