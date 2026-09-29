const previewPolicy =
  "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data: blob:; connect-src 'none'; worker-src 'none'; child-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'";

// Measure at a stable layout viewport, then transform the whole document. Resizing the
// iframe to scrollHeight would feed back into vh/percentage sizing in generated HTML.
const fitDocumentScript = `(() => {
  const start = () => {
    const root = document.documentElement;
    const body = document.body;
    let pending = false;
    const fit = () => {
      pending = false;
      root.style.setProperty('transform', 'none', 'important');
      let left = 0;
      let top = 0;
      let right = Math.max(innerWidth, root.scrollWidth, body.scrollWidth);
      let bottom = Math.max(innerHeight, root.scrollHeight, body.scrollHeight);
      for (const element of body.querySelectorAll('*')) {
        if (!(element instanceof HTMLElement || element instanceof SVGSVGElement)) continue;
        const rect = element.getBoundingClientRect();
        if (!rect.width || !rect.height) continue;
        left = Math.min(left, rect.left);
        top = Math.min(top, rect.top);
        right = Math.max(right, rect.right);
        bottom = Math.max(bottom, rect.bottom);
      }
      const width = right - left;
      const height = bottom - top;
      const scale = Math.min(innerWidth / width, innerHeight / height);
      const x = (innerWidth - width * scale) / 2 - left * scale;
      const y = (innerHeight - height * scale) / 2 - top * scale;
      root.style.setProperty('transform', 'translate(' + x + 'px,' + y + 'px) scale(' + scale + ')', 'important');
    };
    const schedule = () => {
      if (pending) return;
      pending = true;
      requestAnimationFrame(fit);
    };
    const resize = new ResizeObserver(schedule);
    resize.observe(root);
    resize.observe(body);
    new MutationObserver(schedule).observe(body, {
      subtree: true, childList: true, characterData: true, attributes: true,
      attributeFilter: ['style', 'class', 'width', 'height', 'viewBox', 'hidden']
    });
    addEventListener('resize', schedule);
    document.fonts.ready.then(schedule);
    fit();
  };
  if (document.readyState === 'complete') start();
  else addEventListener('load', start, { once: true });
})();`;

export function buildAnimationPreviewDocument(html: string): string {
  return `<!DOCTYPE html><meta http-equiv="Content-Security-Policy" content="${previewPolicy}">${html}<style>html{overflow:hidden!important;transform-origin:0 0!important}body{overflow:visible!important}</style><script>${fitDocumentScript}</script>`;
}
