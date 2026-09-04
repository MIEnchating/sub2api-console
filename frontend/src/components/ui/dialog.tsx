import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { Cancel01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import * as React from "react";

import { cn } from "@/lib/utils";
import { Button } from "./button";

export const dialogContentLayout =
  "bg-popover text-popover-foreground ring-foreground/10 fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100svh-2rem)] w-fit min-w-[min(20rem,calc(100%-2rem))] max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-xl p-4 text-sm ring-1 transition-[opacity,scale] duration-150 ease-out outline-none data-ending-style:scale-95 data-ending-style:opacity-0 data-starting-style:scale-95 data-starting-style:opacity-0";

export const dialogBodyLayout =
  "min-h-0 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain pr-1";

export const dialogWidthLayouts = {
  content: "",
  medium: "w-[min(32rem,calc(100vw-2rem))]",
  wide: "w-[min(64rem,calc(100vw-2rem))]",
  table: "w-[min(90rem,calc(100vw-2rem))]",
} as const;

type DialogContentWidth = keyof typeof dialogWidthLayouts;

export const dialogHeightLayouts = {
  content: "",
  adaptive: "max-h-[calc(100svh-2rem)]",
  medium: "max-h-[min(32rem,calc(100svh-2rem))]",
  large: "max-h-[min(38rem,calc(100svh-2rem))]",
  tall: "max-h-[min(44rem,calc(100svh-2rem))]",
} as const;

type DialogContentHeight = keyof typeof dialogHeightLayouts;

export const compactOperationDialogLayout = {
  width: "medium",
  height: "adaptive",
} as const;

export function operationDialogWidth(
  hasResults: boolean,
  resultWidth: "wide" | "table" = "wide",
): DialogContentWidth {
  return hasResults ? resultWidth : "medium";
}

export function operationDialogHeight(hasResults: boolean): DialogContentHeight {
  return hasResults ? "adaptive" : "content";
}

export function dialogContentClass(
  width: DialogContentWidth = "content",
  height: DialogContentHeight = "content",
  className?: string,
) {
  return cn(dialogContentLayout, dialogWidthLayouts[width], dialogHeightLayouts[height], className);
}

function Dialog(props: DialogPrimitive.Root.Props) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />;
}

function DialogPortal(props: DialogPrimitive.Portal.Props) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogOverlay(props: DialogPrimitive.Backdrop.Props) {
  return (
    <DialogPrimitive.Backdrop
      data-slot="dialog-overlay"
      {...props}
      className={cn(
        "fixed inset-0 isolate z-50 bg-black/10 transition-opacity duration-150 ease-out data-ending-style:opacity-0 data-starting-style:opacity-0 supports-backdrop-filter:backdrop-blur-xs",
        props.className,
      )}
    />
  );
}

function DialogContent(
  props: DialogPrimitive.Popup.Props & {
    showCloseButton?: boolean;
    width?: DialogContentWidth;
    height?: DialogContentHeight;
  },
) {
  const {
    children,
    showCloseButton = true,
    width = "medium",
    height = "content",
    className,
    ...popupProps
  } = props;
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Popup
        data-slot="dialog-content"
        {...popupProps}
        className={
          typeof className === "function"
            ? (state) => dialogContentClass(width, height, className(state))
            : dialogContentClass(width, height, className)
        }
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            render={<Button variant="ghost" className="absolute top-2 right-2" size="icon-sm" />}
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
            <span className="sr-only">关闭</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Popup>
    </DialogPortal>
  );
}

function DialogBody(props: React.ComponentProps<"div">) {
  return (
    <div data-slot="dialog-body" {...props} className={cn(dialogBodyLayout, props.className)} />
  );
}

function DialogHeader(props: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      {...props}
      className={cn("flex flex-col gap-2", props.className)}
    />
  );
}

function DialogFooter(props: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-footer"
      {...props}
      className={cn(
        "bg-muted/50 -mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-xl border-t p-4 sm:flex-row sm:justify-end",
        props.className,
      )}
    />
  );
}

function DialogTitle(props: DialogPrimitive.Title.Props) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      {...props}
      className={cn("text-base leading-none font-medium", props.className)}
    />
  );
}

function DialogDescription(props: DialogPrimitive.Description.Props) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      {...props}
      className={cn("text-muted-foreground text-sm", props.className)}
    />
  );
}

export {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
};
