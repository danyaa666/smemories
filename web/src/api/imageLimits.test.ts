import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { IMAGE_LIMITS } from "./yearbooks";

// T-164 AC2: the numbers in the "image too large" message are the ones the server enforces. They live in internal/media/image.go;
// this test reads them from there, so changing one side without the other fails the build.
const go = readFileSync(resolve(process.cwd(), "../internal/media/image.go"), "utf8"); // vitest runs in web/
const constant = (name: string) => {
  const m = new RegExp(`\\b${name}\\s*=\\s*([0-9_]+)`).exec(go);
  if (!m) throw new Error(`${name} not found in internal/media/image.go`);
  return Number(m[1]!.replaceAll("_", ""));
};

describe("IMAGE_LIMITS match internal/media/image.go", () => {
  it("side, JPEG megapixels, PNG and WebP megapixels", () => {
    expect(IMAGE_LIMITS.side).toBe(constant("maxSide"));
    expect(IMAGE_LIMITS.jpegMp * 1_000_000).toBe(constant("maxPixels"));
    expect(IMAGE_LIMITS.pngMp * 1_000_000).toBe(constant("maxPixelsPNGWebP"));
  });
});
