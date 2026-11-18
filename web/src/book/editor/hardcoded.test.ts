import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// T-182 (requirement 5): no user-facing string of the editor is typed into the JSX; every one comes from `t()` (en.json and vi.json, checked by lint:i18n).
// Looks at the JSX text and at the string literals of the attributes a person reads or hears.
const ROOTS = ["src/book/editor", "src/pages"];
const ATTRS = new Set([
  "aria-label",
  "aria-description",
  "aria-roledescription",
  "title",
  "placeholder",
  "alt",
]);
/** Allowed literal text: no letters (icons, separators, numbers), or a short symbol such as the multiplication sign. */
const hasLetters = (s: string) => /\p{L}/u.test(s);

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f).split("\\").join("/");
    if (statSync(p).isDirectory()) return files(p);
    const own = !p.includes("/pages/") || p.includes("/pages/Editor"); // src/pages: only the editor's route
    return p.endsWith(".tsx") && !p.endsWith(".test.tsx") && own ? [p] : [];
  });
}

function findLiterals(source: string, name = "x.tsx"): string[] {
  const sf = ts.createSourceFile(name, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const found: string[] = [];
  const at = (n: ts.Node) => `${name}:${sf.getLineAndCharacterOfPosition(n.getStart()).line + 1}`;
  const visit = (n: ts.Node) => {
    if (ts.isJsxText(n) && hasLetters(n.text)) found.push(`${at(n)} text "${n.text.trim()}"`);
    if (ts.isJsxAttribute(n) && ATTRS.has(n.name.getText()) && n.initializer) {
      const v = n.initializer;
      const lit = ts.isStringLiteral(v)
        ? v
        : ts.isJsxExpression(v) && v.expression && ts.isStringLiteralLike(v.expression)
          ? v.expression
          : null;
      if (lit && hasLetters(lit.text)) found.push(`${at(n)} ${n.name.getText()}="${lit.text}"`);
    }
    ts.forEachChild(n, visit);
  };
  visit(sf);
  return found;
}

describe("no hard-coded user-facing string in the editor", () => {
  it("the finder sees a text, a label and a title, and ignores icons and expressions", () => {
    const src = `const a = <div title="Hi" aria-label={"Name"}>Hello {x} <b>×</b> <i aria-hidden="true" /></div>;`;
    expect(findLiterals(src).map((s) => s.replace(/^x\.tsx:\d+ /, ""))).toEqual([
      'title="Hi"',
      'aria-label="Name"',
      'text "Hello"',
    ]);
  });
  it("the editor's own screens have none", () => {
    const list = ROOTS.flatMap(files);
    expect(list.length).toBeGreaterThan(15); // the walk found the screens
    const found = list.flatMap((f) => findLiterals(readFileSync(f, "utf8"), f));
    expect(found).toEqual([]);
  });
});
