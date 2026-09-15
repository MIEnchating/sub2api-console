export function OperationErrorDetails(props: { message: string }) {
  return (
    <div
      role="region"
      aria-label="错误详情"
      tabIndex={0}
      className="text-foreground max-h-[min(16rem,35dvh)] overflow-y-auto overscroll-contain rounded-sm pr-1 leading-relaxed whitespace-pre-wrap wrap-anywhere focus-visible:outline-2 focus-visible:outline-offset-2"
      onPointerDown={(event) => event.stopPropagation()}
    >
      {props.message}
    </div>
  );
}
