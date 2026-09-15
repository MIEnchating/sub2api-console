import {
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type ReactElement,
} from "react";
import { FileUp, Upload } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ContentLoading } from "@/components/content-loading";
import { cn } from "@/lib/utils";

export type FileUploadProps = {
  id?: string;
  label: string;
  accept: string;
  description: string;
  fileName?: string;
  busy?: boolean;
  disabled?: boolean;
  onSelect: (file: File) => void;
};

export function FileUpload(props: FileUploadProps): ReactElement {
  const generatedId = useId();
  const id = props.id ?? generatedId;
  const input = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const disabled = props.disabled || props.busy;

  function select(files: File[]): void {
    if (disabled || files.length === 0) return;
    if (files.length > 1) {
      toast.error("每次只能选择一个文件，请分次读取");
      return;
    }
    const file = files[0];
    const accepted = props.accept.split(",").some((entry) => {
      const value = entry.trim().toLowerCase();
      if (!value) return false;
      if (value.startsWith(".")) return file.name.toLowerCase().endsWith(value);
      if (value.endsWith("/*")) return file.type.toLowerCase().startsWith(value.slice(0, -1));
      return file.type.toLowerCase() === value;
    });
    if (props.accept && !accepted) {
      toast.error(`文件格式不支持，请选择 ${props.accept} 文件`);
      return;
    }
    props.onSelect(file);
  }

  function change(event: ChangeEvent<HTMLInputElement>): void {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    select(files);
  }

  function drop(event: DragEvent<HTMLDivElement>): void {
    event.preventDefault();
    setDragging(false);
    select(Array.from(event.dataTransfer.files));
  }

  return (
    <div
      role="group"
      aria-label="文件上传"
      aria-busy={!!props.busy}
      aria-disabled={!!disabled}
      data-slot="file-upload"
      className={cn(
        "flex min-h-16 min-w-0 items-center gap-3 rounded-lg border border-dashed border-input bg-muted/20 p-3 transition-colors",
        dragging && !disabled && "border-primary bg-primary/5",
        disabled && "opacity-60",
      )}
      onDragOver={(event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = disabled ? "none" : "copy";
        if (!disabled) setDragging(true);
      }}
      onDragLeave={(event) => {
        if (
          !(event.relatedTarget instanceof Node) ||
          !event.currentTarget.contains(event.relatedTarget)
        )
          setDragging(false);
      }}
      onDrop={drop}
    >
      <FileUp aria-hidden="true" className="size-5 shrink-0 text-primary" />
      <input
        ref={input}
        id={id}
        type="file"
        aria-label={props.label}
        aria-describedby={`${id}-description`}
        accept={props.accept}
        disabled={disabled}
        className="hidden"
        tabIndex={-1}
        onChange={change}
      />
      <div className="min-w-0 flex-1">
        {props.busy ? (
          <ContentLoading label="正在读取文件" compact className="min-h-5 py-0" />
        ) : (
          <Tooltip>
            <TooltipTrigger
              render={
                <p
                  role="status"
                  aria-label={props.fileName || "未选择文件"}
                  className="truncate text-sm"
                />
              }
            >
              {props.fileName || "未选择文件"}
            </TooltipTrigger>
            <TooltipContent>{props.fileName || "未选择文件"}</TooltipContent>
          </Tooltip>
        )}
        <p
          id={`${id}-description`}
          className="text-xs leading-5 text-muted-foreground wrap-anywhere"
        >
          {props.description}
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        aria-describedby={`${id}-description`}
        onClick={() => input.current?.click()}
      >
        <Upload aria-hidden="true" />
        {props.fileName ? "重新选择" : "选择文件"}
      </Button>
    </div>
  );
}
