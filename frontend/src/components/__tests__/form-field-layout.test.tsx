import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { FormField } from "../../App";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";

describe("form field layout", () => {
  it("does not make the whole field row an implicit select click target", () => {
    const markup = renderToStaticMarkup(
      <FormField label="平台">
        <Select value="sub2api">
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="sub2api">Sub2API</SelectItem>
          </SelectContent>
        </Select>
      </FormField>,
    );

    expect(markup).not.toContain("<label");
    expect(markup).toContain('data-slot="select-trigger"');
  });
});
