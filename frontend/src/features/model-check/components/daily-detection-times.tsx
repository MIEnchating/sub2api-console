import type { ReactElement } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { FieldError } from "@/components/field-error";

export function DailyDetectionTimes(props: {
  times: string[];
  onChange: (times: string[]) => void;
  pending: boolean;
  error?: string;
}): ReactElement {
  return (
    <>
      <div
        className="max-h-64 space-y-2 overflow-y-auto"
        role="group"
        aria-label="每天检测时间列表"
      >
        {props.times.map((time, index) => (
          <div key={index} className="flex items-end gap-2">
            <label className="min-w-0 flex-1 space-y-1 text-sm">
              {index === 0 ? "每天检测时间（北京时间）" : `每天检测时间 ${index + 1}（北京时间）`}
              <Input
                type="time"
                step={60}
                value={time}
                disabled={props.pending}
                aria-invalid={!!props.error}
                onChange={(event) =>
                  props.onChange(
                    props.times.map((item, position) =>
                      position === index ? event.target.value : item,
                    ),
                  )
                }
              />
            </label>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={`删除检测时间 ${index + 1}`}
              disabled={props.pending || props.times.length === 1}
              onClick={() =>
                props.onChange(props.times.filter((_, position) => position !== index))
              }
            >
              <Trash2 aria-hidden="true" />
            </Button>
          </div>
        ))}
      </div>
      <FieldError message={props.error} />
      <Button
        type="button"
        variant="outline"
        disabled={props.pending || props.times.length >= 24}
        onClick={() => props.onChange([...props.times, ""])}
      >
        <Plus aria-hidden="true" />
        添加检测时间
      </Button>
      <p className="text-xs text-muted-foreground">
        每天可设置 1～24 个北京时间点；服务器重启后等待下一个时间点，不补跑错过的检测。
      </p>
    </>
  );
}
