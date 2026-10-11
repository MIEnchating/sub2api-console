import { useState } from "react";
import { createRoot } from "react-dom/client";

import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { TooltipProvider } from "@/components/ui/tooltip";

function TableFixture() {
  const [name, setName] = useState("键盘可查看的完整账号名称");
  const [edited, setEdited] = useState(false);
  return (
    <TooltipProvider>
      <section aria-label="账号表格" style={{ width: "var(--fixture-width, 160px)" }}>
        <Table aria-label="账号">
          <colgroup>
            <col />
            <col style={{ width: 72 }} />
          </colgroup>
          <TableBody>
            <TableRow>
              <TableCell>短名称</TableCell>
              <TableCell overflowTooltip={false}>
                <Button aria-label="编辑短账号">编辑</Button>
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell>{name}</TableCell>
              <TableCell overflowTooltip={false}>
                <Button aria-label="编辑长账号" onClick={() => setEdited(true)}>
                  编辑
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </section>
      <Button onClick={() => setName("短名称二")}>缩短名称</Button>
      <output>{edited ? "已编辑长账号" : "未编辑"}</output>
    </TooltipProvider>
  );
}

createRoot(document.getElementById("table-root")!).render(<TableFixture />);
