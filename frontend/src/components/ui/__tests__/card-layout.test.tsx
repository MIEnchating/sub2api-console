import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "../card";

describe("Card layout", () => {
  it("places header actions on their own row in narrow panels", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>运行记录</CardTitle>
          <CardAction>查看全部运行记录</CardAction>
        </CardHeader>
      </Card>,
    );
    expect(screen.getByText("查看全部运行记录")).toHaveClass(
      "min-w-0",
      "max-w-full",
      "flex-wrap",
      "col-start-1",
    );
    expect(document.querySelector('[data-slot="card-header"]')).toHaveClass(
      "@container/card-header",
    );
  });
  it("allows long titles, descriptions and content to shrink inside a grid track", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>{"group-".repeat(40)}</CardTitle>
          <CardDescription>{"upstream-".repeat(40)}</CardDescription>
        </CardHeader>
        <CardContent>账号明细</CardContent>
      </Card>,
    );
    expect(screen.getByText("group-".repeat(40))).toHaveClass(
      "min-w-0",
      "[overflow-wrap:anywhere]",
    );
    expect(screen.getByText("upstream-".repeat(40))).toHaveClass(
      "min-w-0",
      "[overflow-wrap:anywhere]",
    );
    expect(screen.getByText("账号明细")).toHaveClass("min-w-0");
    expect(document.querySelector('[data-slot="card"]')).toHaveClass("min-w-0");
  });
});
