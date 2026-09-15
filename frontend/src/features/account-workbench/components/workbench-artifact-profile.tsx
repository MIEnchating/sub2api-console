import { useState, type ReactElement } from "react";
import type { WorkbenchExportMetadata } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormField } from "@/components/form-field";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkbenchSourceProfileCreate } from "./workbench-source-profile-create";

export function WorkbenchArtifactProfile(props: {
  artifact: WorkbenchExportMetadata;
  onClose: () => void;
}): ReactElement {
  const [index, setIndex] = useState(1);
  const [selected, setSelected] = useState<number | null>(null);
  if (selected !== null || props.artifact.count === 1)
    return (
      <WorkbenchSourceProfileCreate
        source={{ artifact_id: props.artifact.id, index: selected ?? 0 }}
        onClose={props.onClose}
      />
    );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>选择私有文件中的账号</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid min-w-0 gap-3">
          <p className="text-sm wrap-anywhere">
            文件：{props.artifact.id}；共 {props.artifact.count} 个账号
          </p>
          <FormField label="账号序号" htmlFor="artifact-profile-index">
            <Input
              id="artifact-profile-index"
              type="number"
              min={1}
              max={props.artifact.count}
              value={Number.isFinite(index) ? index : ""}
              onChange={(event) => setIndex(event.target.valueAsNumber)}
            />
          </FormField>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            返回文件列表
          </Button>
          <Button
            disabled={!Number.isInteger(index) || index < 1 || index > props.artifact.count}
            onClick={() => setSelected(index - 1)}
          >
            核对所选账号身份
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
