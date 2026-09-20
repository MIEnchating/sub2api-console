import { useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { MoreHorizontal } from "lucide-react";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

function SharedControlsLayout(): ReactElement {
  const surface = new URLSearchParams(window.location.search).get("surface");
  const [selected, setSelected] = useState(["account"]);
  const [result, setResult] = useState("");
  if (surface === "selection") {
    return (
      <MultiSelect
        title="账号筛选"
        options={[{ value: "account", label: "upstream-account-".repeat(20) }]}
        selected={selected}
        onChange={setSelected}
      />
    );
  }
  if (surface === "card") {
    return (
      <Card>
        <CardHeader>
          <CardTitle>自动巡检运行记录</CardTitle>
          <CardAction>
            <Button>查看全部运行记录</Button>
            <Button>查看详细健康统计</Button>
            <Button>重新读取任务状态</Button>
          </CardAction>
        </CardHeader>
      </Card>
    );
  }
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button aria-label="更多操作" size="icon" />}>
          <MoreHorizontal aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          {surface === "menu-long" ? (
            <DropdownMenuItem onClick={() => setResult("已选择账号")}>
              {"upstream-account-".repeat(20)}
            </DropdownMenuItem>
          ) : (
            Array.from({ length: 16 }, (_, index) => (
              <DropdownMenuItem key={index} onClick={() => setResult(`已执行操作 ${index + 1}`)}>
                操作 {index + 1}
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <p role="status">{result}</p>
    </>
  );
}

createRoot(document.getElementById("controls-root")!).render(<SharedControlsLayout />);
