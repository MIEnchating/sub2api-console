import { useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import type { AnimationResult } from "@/api";

const previewPolicy =
  "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data: blob:; connect-src 'none'; worker-src 'none'; child-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'";
const canvasWidth = 1024;
const canvasHeight = 768;
// Keep the main SVG inside the preview viewport, including fixed-size generated SVGs.
// This affects only the preview; the original HTML remains available in the code tab.
const previewStyle = `html,body{width:100%!important;height:100%!important;overflow:hidden!important}body{box-sizing:border-box!important;margin:0!important;max-width:100%!important;min-width:0!important}svg:not(svg svg){max-width:100%!important;max-height:100%!important}`;

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
    return `<!DOCTYPE html><meta http-equiv="Content-Security-Policy" content="${previewPolicy}">${props.result.html}<style>${previewStyle}</style>`;
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
