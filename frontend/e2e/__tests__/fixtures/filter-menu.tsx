import { useState } from "react";
import { createRoot } from "react-dom/client";

import { FilterMenu } from "@/components/data-table/filter-menu";

const options = ["默认", "Codex", "Claude"];

function FilterFixture() {
  const [value, setValue] = useState<string | null>(null);
  return <FilterMenu label="分组" options={options} value={value} onValueChange={setValue} />;
}

createRoot(document.getElementById("filter-root")!).render(<FilterFixture />);
