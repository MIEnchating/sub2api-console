import { Popover } from "@base-ui/react/popover";
import { ListChecks } from "lucide-react";
import type { ReactElement } from "react";
import type { PrecheckQuestionID } from "@/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { allPrecheckQuestions, precheckQuestionLabels } from "../constants";

export function PrecheckQuestionSelector(props: {
  value: PrecheckQuestionID[];
  onChange: (value: PrecheckQuestionID[]) => void;
  disabled?: boolean;
}): ReactElement {
  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <Button
            type="button"
            variant="outline"
            disabled={props.disabled}
            aria-label="选择前置检测题目"
          />
        }
      >
        <ListChecks aria-hidden="true" />
        检测题目（{props.value.length}）
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner sideOffset={4} align="start" collisionPadding={8} className="z-50">
          <Popover.Popup
            aria-label="前置检测题目"
            className="w-56 max-w-(--available-width) rounded-lg border bg-popover p-3 text-popover-foreground shadow-lg outline-none"
          >
            <label className="flex items-center gap-2 border-b pb-2 text-sm">
              <Checkbox
                checked={props.value.length === allPrecheckQuestions.length}
                indeterminate={
                  props.value.length > 0 && props.value.length < allPrecheckQuestions.length
                }
                disabled={props.disabled}
                onCheckedChange={(checked) =>
                  props.onChange(checked ? [...allPrecheckQuestions] : [])
                }
              />
              全选
            </label>
            {allPrecheckQuestions.map((id) => (
              <label key={id} className="flex items-center gap-2 pt-2 text-sm">
                <Checkbox
                  checked={props.value.includes(id)}
                  disabled={props.disabled}
                  onCheckedChange={(checked) =>
                    props.onChange(
                      allPrecheckQuestions.filter((question) =>
                        question === id ? checked : props.value.includes(question),
                      ),
                    )
                  }
                />
                {precheckQuestionLabels[id]}
              </label>
            ))}
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
