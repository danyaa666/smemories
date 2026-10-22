import { describe, expect, it } from "vitest";
import fixtureText from "../../internal/signature/testdata/canonical.json?raw";
import {
  MAX_BYTES,
  addPoint,
  canonical,
  fit,
  parseSignature,
  quantise,
  signaturePath,
  simplify,
  within,
  type Stroke,
} from "./signature";

const lim = { maxStrokes: 12, maxPoints: 600 };
// The file the Go validator test reads too (internal/signature/fixture_test.go): one JSON file passes both.
const fixture = JSON.parse(fixtureText) as {
  valid: { name: string; canonical: string }[];
  invalid: { name: string; json: string }[];
};

describe("shared canonical fixture", () => {
  it.each(fixture.valid.map((c) => [c.name, c.canonical] as const))(
    "%s: accepted, and serialised back to the identical string",
    (_name, text) => {
      const sig = parseSignature(JSON.parse(text));
      expect(sig).not.toBeNull();
      expect(canonical(sig!.s)).toBe(text);
      expect(text.length).toBeLessThanOrEqual(MAX_BYTES);
    },
  );
  it.each(fixture.invalid.map((c) => [c.name, c.json] as const))("%s: refused", (_name, text) => {
    expect(parseSignature(JSON.parse(text))).toBeNull();
    expect(signaturePath(JSON.parse(text))).toBeNull();
  });
  it("covers the byte cap from both sides", () => {
    expect(Math.max(...fixture.valid.map((c) => c.canonical.length))).toBeGreaterThan(3000);
    expect(fixture.invalid.some((c) => c.json.length > MAX_BYTES)).toBe(true);
  });
});

describe("quantise and addPoint", () => {
  it("rounds to integers and stays inside the space", () => {
    expect(quantise(250.4, 0, 500, 1000)).toBe(501); // 500.8
    expect(quantise(160, 10, 300, 400)).toBe(200);
    expect(quantise(-5, 0, 500, 1000)).toBe(0);
    expect(quantise(900, 0, 500, 1000)).toBe(1000);
  });
  it("keeps points at least 2 units apart", () => {
    const s: Stroke = [];
    const added = [
      [0, 0],
      [1, 1],
      [2, 0],
      [2, 1],
      [4, 0],
    ].map(([x, y]) => addPoint(s, x!, y!));
    expect(added).toEqual([true, false, true, false, true]);
    expect(s).toEqual([0, 0, 2, 0, 4, 0]);
  });
});

describe("simplify (Ramer-Douglas-Peucker)", () => {
  it("drops points on the line and keeps the ends", () => {
    expect(simplify([0, 0, 10, 0, 20, 0, 30, 0], 1.5)).toEqual([0, 0, 30, 0]);
  });
  it("drops a bump of 1 unit and keeps a bump of 3 at epsilon 1.5; a larger epsilon drops the 3", () => {
    expect(simplify([0, 0, 10, 1, 20, 0], 1.5)).toEqual([0, 0, 20, 0]);
    expect(simplify([0, 0, 10, 3, 20, 0], 1.5)).toEqual([0, 0, 10, 3, 20, 0]);
    expect(simplify([0, 0, 10, 3, 20, 0], 4)).toEqual([0, 0, 20, 0]);
  });
  it("turns a tap into a two-point dot and keeps a closed loop", () => {
    expect(simplify([5, 5], 1.5)).toEqual([5, 5, 5, 5]);
    expect(simplify([0, 0, 50, 0, 50, 50, 0, 50, 0, 0], 1.5)).toEqual([
      0, 0, 50, 0, 50, 50, 0, 50, 0, 0,
    ]);
  });
});

// A line of 3-digit coordinates whose y wanders by `wobble(i)`: the byte cap (about 380 such points) binds long before 600 points.
const line = (n: number, wobble: (i: number) => number): Stroke => {
  const s: Stroke = [];
  for (let i = 0; i < n; i++) s.push(100 + i, 200 + wobble(i));
  return s;
};

describe("fit: the 12 / 600 / 3072-byte limits", () => {
  it("keeps a small drawing at epsilon 1.5", () => {
    const raw = [[0, 0, 10, 4, 20, 0, 30, 4]];
    expect(fit(raw, lim)).toEqual(raw.map((s) => simplify(s, 1.5)));
  });
  it("widens epsilon when the first pass is over the byte cap, and ends within every limit", () => {
    const raw = [line(580, (i) => ((i * 7) % 13) - 6)]; // 4320 bytes at epsilon 1.5, 3600 at 2, 2880 at 3
    expect(
      within(
        raw.map((s) => simplify(s, 1.5)),
        lim,
      ),
    ).toBe(false);
    const got = fit(raw, lim);
    expect(got).not.toBeNull();
    expect(within(got!, lim)).toBe(true);
    expect(canonical(got!).length).toBeLessThanOrEqual(MAX_BYTES);
  });
  it("refuses (null) what even epsilon 4 cannot fit", () => {
    expect(fit([line(600, (i) => (i % 2 ? 20 : -20))], lim)).toBeNull();
  });
  it("refuses more strokes or points than the API allows", () => {
    const stroke = [0, 0, 100, 100];
    expect(fit(Array(13).fill(stroke), lim)).toBeNull();
    expect(fit(Array(12).fill(stroke), lim)).not.toBeNull();
    expect(fit([[0, 0, 10, 10, 20, 0, 30, 10]], { maxStrokes: 12, maxPoints: 3 })).toBeNull();
  });
});

describe("signaturePath", () => {
  const sig = (s: unknown) => ({ v: 1, w: 1000, h: 400, s });
  it("builds the path from integers only", () => {
    expect(
      signaturePath(
        sig([
          [0, 0, 50, 0],
          [100, 100, 100, 100],
        ]),
      ),
    ).toBe("M0 0L50 0M100 100L100 100");
  });
  it.each([
    ["a float", sig([[0, 0, 50.5, 0]])],
    ["a string number", sig([[0, 0, "50", 0]])],
    ["markup in a number slot", sig([[0, 0, '5"/><script>', 0]])],
    ["NaN", sig([[0, 0, NaN, 0]])],
    ["Infinity", sig([[0, 0, Infinity, 0]])],
    ["null", sig([[0, 0, null, 0]])],
    ["x out of range", sig([[0, 0, 1001, 0]])],
    ["y out of range", sig([[0, 0, 50, 401]])],
    ["negative", sig([[-1, 0, 50, 0]])],
    ["an extra key", { ...sig([[0, 0, 50, 0]]), t: [1] }],
    ["strokes not an array", sig("M0 0")],
    ["null value", null],
    ["a string", "M0 0L50 0"],
  ])("renders nothing for %s", (_n, value) => {
    expect(signaturePath(value)).toBeNull();
  });
});
