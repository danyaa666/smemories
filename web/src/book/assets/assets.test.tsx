import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { checkAssets, checkSvg } from "../../../scripts/check-assets.mjs";
import ContactSheet from "./ContactSheet";
import { AssetSvg, assets, getAsset, loadSvg } from "./index";

const DIR = resolve(__dirname);
const read = (rel: string) => readFileSync(resolve(DIR, rel), "utf8");

describe("registry", () => {
  it("has 24 authored and 24 curated assets of the three kinds", () => {
    expect(assets.length).toBeGreaterThanOrEqual(48);
    expect(assets.filter((a) => a.licence === "authored").length).toBeGreaterThanOrEqual(24);
    expect(assets.filter((a) => a.licence !== "authored").length).toBeGreaterThanOrEqual(24);
    expect(new Set(assets.map((a) => a.kind))).toEqual(new Set(["sticker", "shape", "frame"]));
  });

  it("lists the same ids, in the same bytes, as the copy the Go server embeds (internal/assets)", () => {
    const goCopy = resolve(DIR, "../../../../internal/assets/registry.json");
    expect(readFileSync(goCopy, "utf8")).toBe(read("registry.json"));
    expect(
      JSON.parse(readFileSync(goCopy, "utf8")).assets.map((a: { id: string }) => a.id),
    ).toEqual(assets.map((a) => a.id));
  });

  it("passes the pack check", () => {
    expect(checkAssets(DIR)).toEqual([]);
  });

  it("getAsset finds a listed id and nothing else", () => {
    expect(getAsset("shape.circle")?.kind).toBe("shape");
    expect(getAsset("shape.nope")).toBeUndefined();
    expect(getAsset("")).toBeUndefined();
  });
});

describe("pack check", () => {
  const dirs: string[] = [];
  afterEach(() => {
    while (dirs.length) rmSync(dirs.pop() as string, { recursive: true, force: true });
  });
  function copy() {
    const d = mkdtempSync(join(tmpdir(), "assets-"));
    dirs.push(d);
    cpSync(DIR, d, {
      recursive: true,
      filter: (s) => !/\.(tsx?|json)$/.test(s) || s.endsWith("registry.json"),
    });
    return d;
  }

  it("fails on a planted <script>", () => {
    const d = copy();
    const f = join(d, "svg", "shape.circle.svg");
    writeFileSync(f, readFileSync(f, "utf8").replace("</svg>", "<script>alert(1)</script></svg>"));
    expect(checkAssets(d)).toEqual(["shape.circle: element <script> not allowed"]);
  });

  it("fails on an entry without licence data, on a missing licence file and on an unlisted svg", () => {
    const d = copy();
    const reg = JSON.parse(readFileSync(join(d, "registry.json"), "utf8"));
    reg.assets[0].licence = "";
    reg.assets[1].licence_file = "licences/missing.txt";
    writeFileSync(join(d, "registry.json"), JSON.stringify(reg));
    writeFileSync(
      join(d, "svg", "sticker.stray.svg"),
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0"/></svg>',
    );
    const problems = checkAssets(d).join("\n");
    expect(problems).toContain(`${reg.assets[0].id}: licence must be one of`);
    expect(problems).toContain(`${reg.assets[1].id}: licence_file missing or not found`);
    expect(problems).toContain("svg/sticker.stray.svg: not listed in registry.json");
  });

  it("rejects each unsafe construct", () => {
    const ok =
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0H5V5Z"/></svg>';
    expect(checkSvg(ok)).toEqual([]);
    const bad = [
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><foreignObject/></svg>',
        "foreignObject",
      ],
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path onclick="x()" d="M0 0"/></svg>',
        "event attribute",
      ],
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><image href="http://x/a.png"/></svg>',
        "not allowed",
      ],
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path fill="url(http://x/#a)" d="M0 0"/></svg>',
        "url()",
      ],
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><style>@import "x";</style></svg>',
        "@import",
      ],
      [
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path style="fill:red" d="M0 0"/></svg>',
        "attribute style",
      ],
      ['<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>', "viewBox"],
      ['<!DOCTYPE svg><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>', "doctype"],
      [ok.replace("M0 0H5V5Z", "M0 0".repeat(6000)), "bytes"],
    ] as const;
    for (const [svg, want] of bad)
      expect(checkSvg(svg).join("|"), svg.slice(0, 80)).toContain(want);
  });
});

/** True when the element draws something: a path with numbers, a circle or rect with a size, a polygon with points. */
function draws(el: Element): boolean {
  const n = (a: string) => Number(el.getAttribute(a));
  switch (el.localName) {
    case "path":
      return /[0-9]/.test(el.getAttribute("d") ?? "");
    case "circle":
      return n("r") > 0;
    case "ellipse":
      return n("rx") > 0 && n("ry") > 0;
    case "rect":
      return n("width") > 0 && n("height") > 0;
    case "line":
      return (
        el.getAttribute("x1") !== el.getAttribute("x2") ||
        el.getAttribute("y1") !== el.getAttribute("y2")
      );
    case "polyline":
    case "polygon":
      return (el.getAttribute("points") ?? "").trim().split(/[\s,]+/).length >= 4;
    case "g":
      return Array.from(el.children).some(draws);
    default:
      return false;
  }
}

describe("AssetSvg", () => {
  it("renders every asset as inline elements with something to draw", async () => {
    for (const a of assets) {
      const { container, unmount } = render(<AssetSvg id={a.id} label={a.id} />);
      await waitFor(() => expect(container.querySelector("svg")).not.toBeNull(), { timeout: 5000 });
      const svg = container.querySelector("svg") as SVGElement;
      expect(svg.getAttribute("viewBox"), a.id).toBe(`0 0 ${a.width} ${a.height}`);
      expect(Array.from(svg.children).some(draws), `${a.id} draws nothing`).toBe(true);
      expect(svg.querySelector("script,foreignObject,image,style"), a.id).toBeNull();
      unmount();
    }
  }, 60_000);

  it("tints only a tintable asset", async () => {
    const tintable = assets.find((a) => a.tintable)!;
    const fixed = assets.find((a) => !a.tintable)!;
    const t = render(<AssetSvg id={tintable.id} tint="rgb(1, 2, 3)" />);
    await waitFor(() => expect(t.container.querySelector("svg")).not.toBeNull());
    expect((t.container.querySelector("svg") as SVGElement).style.color).toBe("rgb(1, 2, 3)");
    const f = render(<AssetSvg id={fixed.id} tint="rgb(1, 2, 3)" />);
    await waitFor(() => expect(f.container.querySelector("svg")).not.toBeNull());
    expect((f.container.querySelector("svg") as SVGElement).style.color).toBe("");
  });

  it("is decorative without a label and named with one", async () => {
    const a = render(<AssetSvg id="shape.circle" />);
    await waitFor(() => expect(a.container.querySelector("svg")).not.toBeNull());
    expect(a.container.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
    const b = render(<AssetSvg id="shape.circle" label="A circle" />);
    expect(await b.findByRole("img", { name: "A circle" })).toBeTruthy();
  });

  it("draws nothing for an id that is not in the registry", async () => {
    const { container } = render(<AssetSvg id="sticker.unknown" />);
    expect(await loadSvg("sticker.unknown")).toBeUndefined();
    expect(container.innerHTML).toBe("");
  });
});

describe("ContactSheet", () => {
  it("has a cell for every asset, plain and tinted", async () => {
    const { container } = render(<ContactSheet />);
    expect(screen.getAllByTestId("asset-cell")).toHaveLength(assets.length);
    await waitFor(() => expect(container.querySelectorAll("svg")).toHaveLength(assets.length * 2), {
      timeout: 10_000,
    });
  }, 30_000);
});
