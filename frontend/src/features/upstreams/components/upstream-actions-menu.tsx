import { Ban, FolderOpen, MoreHorizontal, RotateCcw } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { UpstreamExclusionDialog } from "./upstream-exclusion";

type Props = {
  host: string;
  excluded: boolean;
  onViewGroups: () => void;
  children?: ReactNode;
};

export function UpstreamActionsMenu(props: Props) {
  const [open, setOpen] = useState(false);
  const [stopOnly, setStopOnly] = useState(false);
  const [mounted, setMounted] = useState(false);
  const [session, setSession] = useState(0);
  const [active, setActive] = useState(false);
  const launch = (stop: boolean): void => {
    if (!active) {
      setStopOnly(stop);
      setSession((value) => value + 1);
    }
    setMounted(true);
    setOpen(true);
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="outline"
              size="icon"
              className="data-popup-open:bg-muted"
              aria-label="更多操作"
            />
          }
        >
          <MoreHorizontal className="size-4" aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuItem onClick={props.onViewGroups}>
            <FolderOpen aria-hidden="true" />
            查看分组
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => launch(false)}>
            {props.excluded ? <RotateCcw aria-hidden="true" /> : <Ban aria-hidden="true" />}
            {props.excluded ? "取消排除上游" : "排除上游"}
          </DropdownMenuItem>
          {props.excluded && (
            <DropdownMenuItem onClick={() => launch(true)}>
              <Ban aria-hidden="true" />
              关闭已排除上游的账号调度
            </DropdownMenuItem>
          )}
          {props.children}
        </DropdownMenuContent>
      </DropdownMenu>
      {mounted && (
        <UpstreamExclusionDialog
          key={session}
          host={props.host}
          excluded={props.excluded}
          open={open}
          stopOnly={stopOnly}
          onActiveChange={setActive}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
