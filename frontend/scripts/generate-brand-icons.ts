import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

// Run with: bun scripts/generate-brand-icons.ts
// console-mark.svg is the only source; render on an empty, offline browser page.
const publicDirectory = new URL("../public/", import.meta.url);
const svg = await readFile(new URL("console-mark.svg", publicDirectory), "utf8");
const browser = await chromium.launch({ headless: true });

function createIco(images: Map<number, Buffer>): Buffer {
  const header = Buffer.alloc(6 + images.size * 16);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(images.size, 4);
  let offset = header.length;
  let index = 0;
  for (const [size, png] of images) {
    const entry = 6 + index * 16;
    header.writeUInt8(size < 256 ? size : 0, entry);
    header.writeUInt8(size < 256 ? size : 0, entry + 1);
    header.writeUInt16LE(1, entry + 4);
    header.writeUInt16LE(32, entry + 6);
    header.writeUInt32LE(png.length, entry + 8);
    header.writeUInt32LE(offset, entry + 12);
    offset += png.length;
    index += 1;
  }
  return Buffer.concat([header, ...images.values()]);
}

try {
  const context = await browser.newContext({ offline: true });
  const page = await context.newPage();
  const sizes = [16, 32, 48, 180, 192, 512];
  const rendered = await page.evaluate(
    async (options: { svg: string; sizes: number[] }): Promise<[number, string][]> => {
      const image = new Image();
      image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(options.svg)}`;
      await image.decode();
      return options.sizes.map((size): [number, string] => {
        const source = document.createElement("canvas");
        source.width = size * 4;
        source.height = size * 4;
        const sourceContext = source.getContext("2d");
        if (!sourceContext) throw new Error("无法创建图标绘制上下文");
        sourceContext.drawImage(image, 0, 0, source.width, source.height);
        const output = document.createElement("canvas");
        output.width = size;
        output.height = size;
        const outputContext = output.getContext("2d");
        if (!outputContext) throw new Error("无法创建图标缩放上下文");
        outputContext.imageSmoothingEnabled = true;
        outputContext.imageSmoothingQuality = "high";
        outputContext.drawImage(source, 0, 0, size, size);
        return [size, output.toDataURL("image/png").split(",")[1]];
      });
    },
    { svg, sizes },
  );
  const pngs = new Map(rendered.map(([size, encoded]) => [size, Buffer.from(encoded, "base64")]));
  const outputs: [number, string][] = [
    [32, "favicon-32x32.png"],
    [180, "apple-touch-icon.png"],
    [192, "icon-192.png"],
    [512, "icon-512.png"],
  ];
  for (const [size, filename] of outputs) {
    const png = pngs.get(size);
    if (!png) throw new Error(`图标尺寸 ${size} 未生成`);
    await writeFile(new URL(filename, publicDirectory), png);
  }
  const icoImages = new Map<number, Buffer>();
  for (const size of [16, 32, 48]) {
    const png = pngs.get(size);
    if (!png) throw new Error(`ICO 尺寸 ${size} 未生成`);
    icoImages.set(size, png);
  }
  await writeFile(new URL("favicon.ico", publicDirectory), createIco(icoImages));
  process.stdout.write(`已从 SVG 生成图标：${fileURLToPath(publicDirectory)}\n`);
} finally {
  await browser.close();
}
