import { describe, expect, it } from "vitest";
import i18n from "../../i18n";
import en from "../../locales/en.json";

// T-182 (requirement 5): every counted message of the editor reads right for 0, 1, 2, 5 and 21 items in English (one/other) and in Vietnamese (no plural form: one text for all).
function counted(o: Record<string, unknown>, path = ""): string[] {
  return Object.entries(o).flatMap(([k, v]) =>
    v && typeof v === "object"
      ? counted(v as Record<string, unknown>, path ? `${path}.${k}` : k)
      : k.endsWith("_one")
        ? [`${path}.${k.slice(0, -4)}`]
        : [],
  );
}

describe("counted messages", () => {
  const keys = counted(en.editor, "editor");
  it("finds the counted messages", () => expect(keys.length).toBeGreaterThanOrEqual(8));
  for (const lng of ["en", "vi"]) {
    it(`${lng}: a count of 0, 1, 2, 5 and 21 gives a finished sentence (no key, no placeholder left)`, async () => {
      const t = i18n.getFixedT(lng);
      for (const key of keys)
        for (const count of [0, 1, 2, 5, 21]) {
          const text = t(key, { count, what: "x", where: "x", name: "x" });
          expect(text, `${lng} ${key} (${count})`).not.toBe(key);
          expect(text).not.toContain("{{");
        }
    });
  }
  it("English says 1 item and 2 items", () => {
    const t = i18n.getFixedT("en");
    expect(t("editor.history.items", { count: 1 })).toBe("1 item");
    expect(t("editor.history.items", { count: 2 })).toBe("2 items");
    expect(i18n.getFixedT("vi")("editor.history.items", { count: 1 })).toBe("1 mục");
  });
});
