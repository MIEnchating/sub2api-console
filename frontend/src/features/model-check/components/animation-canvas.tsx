import { useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { buildAnimationPreviewDocument } from "../lib/animation-preview-document";

const canvasWidth = 1024;
const canvasHeight = 768;

export function AnimationCanvas(props: {
  result: AnimationResult;
  thumbnail?: boolean;
}): ReactElement {
  const hostRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const document = useMemo(() => {
    if (!props.result.html) return undefined;
    // Keep the generated document in an opaque-origin sandbox, allow inline animation scripts,
    // and block network access through the document CSP.
    return buildAnimationPreviewDocument(props.result.html);
  }, [props.result.html]);
  useEffect(() => {
    const host = hostRef.current;
    if (!host || !document) return;
    const observer = new ResizeObserver(([entry]) => {
      setSize({ width: entry.contentRect.width, height: entry.contentRect.height });
    });
    observer.observe(host);
    return () => observer.disconnect();
  }, [document]);
  const alt = `${props.result.account_name}生成的鹈鹕骑自行车动画`;
  if (document) {
    const scale = Math.min(size.width / canvasWidth, size.height / canvasHeight);
    return (
      <div ref={hostRef} className="relative h-full min-h-0 w-full overflow-hidden bg-white">
        <iframe
          aria-label={alt}
          srcDoc={document}
          sandbox="allow-scripts"
          scrolling="no"
          referrerPolicy="no-referrer"
          tabIndex={props.thumbnail ? -1 : 0}
          aria-hidden={props.thumbnail || undefined}
          className={
            props.thumbnail
              ? "pointer-events-none absolute border-0 bg-white"
              : "absolute border-0 bg-white"
          }
          style={{
            width: canvasWidth,
            height: canvasHeight,
            transformOrigin: "top left",
            transform: `scale(${scale})`,
            left: (size.width - canvasWidth * scale) / 2,
            top: (size.height - canvasHeight * scale) / 2,
          }}
        />
      </div>
    );
  }
  return (
    <img
      className="h-full min-h-0 w-full object-contain"
      src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(props.result.svg ?? "")}`}
      alt={alt}
    />
  );
}
