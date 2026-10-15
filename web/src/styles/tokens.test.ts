import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const read = (p: string) => readFileSync(resolve(__dirname, p), "utf8");

describe("design tokens", () => {
  it("equal the design's file apart from the Google Fonts @import", () => {
    const design = read("../../../docs/design/notes/tokens.css");
    const ported = read("tokens.css");
    expect(design).toMatch(/^@import url\('https:\/\/fonts\.googleapis\.com/m);
    expect(design.replace(/^@import url\([^)]*\);\n\n/m, "")).toBe(ported);
  });

  it("fonts are self-hosted: no third-party font host in any source file", () => {
    const walk = (dir: string): string[] =>
      readdirSync(dir).flatMap((f) => {
        const p = join(dir, f);
        if (p.includes("/spike/")) return []; // the print spike is dev-only and not built
        return statSync(p).isDirectory() ? walk(p) : /\.(css|tsx?)$/.test(f) ? [p] : [];
      });
    const offenders = [...walk(resolve(__dirname, "..")), resolve(__dirname, "../../index.html")]
      .filter((p) => !p.endsWith("tokens.test.ts"))
      .filter((p) => /fonts\.(googleapis|gstatic)\.com/.test(readFileSync(p, "utf8")));
    expect(offenders).toEqual([]);
  });

  it("loads only the latin, latin-ext and vietnamese subsets of Fraunces", () => {
    const css = read("fonts.css");
    const files = [...css.matchAll(/fraunces-([a-z-]+)-wght-normal\.woff2/g)].map((m) => m[1]);
    expect(files.sort()).toEqual(["latin", "latin-ext", "vietnamese"]);
    expect(css).toContain("@fontsource/be-vietnam-pro/400.css");
    expect(css).toContain("@fontsource/patrick-hand/400.css");
  });
});
