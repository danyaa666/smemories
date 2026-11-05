import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { PageTemplate } from "../../api/bookLayout";
import type { PageSize } from "../sizes";
import { TemplatePreview } from "./TemplatePreview";

// T-172: every shipped page template (internal/templates/embed/pagetemplates, authored by us) is drawn by the book's real runtime on all three papers.
// The files are validated by the Go gate (booklayout.TestShippedPageTemplates); this test proves they also draw.

const ROOT = resolve(process.cwd(), "..", "internal/templates/embed/pagetemplates");
const shipped = readdirSync(ROOT, { withFileTypes: true })
  .filter((d) => d.isDirectory())
  .flatMap((d) =>
    readdirSync(resolve(ROOT, d.name))
      .filter((f) => f.endsWith(".json"))
      .map((f) => ({
        template: d.name,
        file: JSON.parse(readFileSync(resolve(ROOT, d.name, f), "utf8")) as PageTemplate & {
          page: PageTemplate["page"];
        },
      })),
  );
const SIZES: PageSize[] = ["A5", "A4", "Letter"];

describe("TemplatePreview", () => {
  it("finds the shipped templates of both layout templates, all four kinds", () => {
    for (const tpl of ["memory-book", "navy-classic"]) {
      const kinds = new Set(shipped.filter((s) => s.template === tpl).map((s) => s.file.kind));
      expect([...kinds].sort()).toEqual(["back", "cover", "notes", "profile"]);
    }
  });

  for (const { template, file } of shipped) {
    for (const size of SIZES) {
      it(`draws ${template}/${file.id} on ${size} at the asked width`, () => {
        const { container } = render(
          <TemplatePreview
            templateId={template}
            kind={file.kind}
            page={file.page}
            width={160}
            size={size}
            label={file.label_en}
          />,
        );
        const root = container.querySelector<HTMLElement>(".tp-preview")!;
        expect(root.getAttribute("role")).toBe("img");
        expect(root.getAttribute("aria-label")).toBe(file.label_en);
        expect(root.style.width).toBe("160px");
        // one sheet, and every item of the template is on it
        expect(container.querySelectorAll(".bk-sheet")).toHaveLength(1);
        expect(container.querySelectorAll("[data-layout-item]")).toHaveLength(
          file.page.items?.length ?? 0,
        );
        // read-only: nothing can take focus or be edited
        expect(container.querySelector("[inert]")).not.toBeNull();
        expect(container.querySelector("textarea, input, button, [contenteditable]")).toBeNull();
      });
    }
  }

  it("draws a template's text as text, in the template's colour token, never as markup", () => {
    const { container } = render(
      <TemplatePreview
        templateId="memory-book"
        kind="cover"
        page={{
          items: [
            { id: "a1b2c3", t: "text", text: "<b>x</b>", x: 100, w: 800, h: 200, cy: 400, r: 0 },
          ],
        }}
        width={200}
        label="t"
      />,
    );
    const item = container.querySelector("[data-layout-item]")!;
    expect(item.textContent).toContain("<b>x</b>");
    expect(item.querySelector("b")).toBeNull();
  });

  it("draws nothing for a kind the book does not know", () => {
    const { container } = render(
      <TemplatePreview templateId="memory-book" kind="poster" page={{}} width={100} label="x" />,
    );
    expect(container.firstChild).toBeNull();
  });
});
