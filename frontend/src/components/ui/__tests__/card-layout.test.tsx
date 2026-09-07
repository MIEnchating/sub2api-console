import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../card";

describe("Card layout", () => {
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
