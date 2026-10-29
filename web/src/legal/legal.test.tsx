import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user } from "../test/api";
import { LegalBlocks, LegalView } from "./LegalPage";
import { TERMS_VERSION, legalDoc, parseLegal } from "./legal";

const signedOut = err(401, "unauthenticated");
const main = () => within(screen.getByRole("main"));
const where = () => screen.getByTestId("where").textContent;
const me = (terms: object) => ok({ ...user(), terms });
const OLD = { accepted_version: "2020-01-01", current_version: TERMS_VERSION, required: true };
const ACCEPTED = {
  accepted_version: TERMS_VERSION,
  current_version: TERMS_VERSION,
  required: false,
};

describe("legal texts", () => {
  it("T-131: every text says what a class admin sees, and all four stay drafts until the owner finalises them", () => {
    for (const name of ["terms", "privacy"] as const) {
      for (const lang of ["en", "vi"] as const) {
        const d = legalDoc(name, lang);
        expect(d.draft).toBe(true);
        const text = JSON.stringify(d.blocks);
        expect(text).toMatch(lang === "en" ? /class admin/i : /quản trị viên lớp/i);
      }
    }
  });

  it("the four documents carry the version the API enforces (internal/legal/legal.go)", () => {
    const go = readFileSync(resolve(process.cwd(), "../internal/legal/legal.go"), "utf8");
    expect(/CurrentVersion = "([^"]+)"/.exec(go)?.[1]).toBe(TERMS_VERSION);
    for (const name of ["terms", "privacy"] as const) {
      for (const lang of ["en", "vi"] as const) {
        expect(legalDoc(name, lang).version).toBe(TERMS_VERSION);
      }
    }
  });

  it("parses headings, paragraphs, bullet lists and bold, and the front matter", () => {
    const d = parseLegal(
      "---\nversion: 1\nstatus: DRAFT - x: y\n---\n\n# Title\n\nOne line\nsecond line\n\n- a\n- b\n  wrapped\n\n## Sub\n**Bold** text\n",
    );
    expect(d).toMatchObject({ version: "1", status: "DRAFT - x: y", draft: true });
    expect(d.blocks).toEqual([
      { kind: "heading", level: 1, text: "Title" },
      { kind: "paragraph", text: "One line second line" },
      { kind: "list", items: ["a", "b wrapped"] },
      { kind: "heading", level: 2, text: "Sub" },
      { kind: "paragraph", text: "**Bold** text" },
    ]);
  });

  it("is a draft while the status starts with DRAFT or BẢN NHÁP, and not otherwise", () => {
    expect(parseLegal("---\nstatus: BẢN NHÁP - x\n---\n").draft).toBe(true);
    expect(parseLegal("---\nstatus: FINAL\n---\n").draft).toBe(false);
    expect(parseLegal("# no front matter").draft).toBe(false);
  });

  it("builds elements and escapes everything else", () => {
    const { container } = render(
      <LegalBlocks
        blocks={
          parseLegal(
            '# <img src=x onerror="alert(1)">\n\n- **bold** <script>alert(1)</script>\n\n<b>raw</b> & more',
          ).blocks
        }
      />,
    );
    expect(container.querySelector("script, img, b")).toBeNull();
    expect(container.querySelector("h1")?.textContent).toBe('<img src=x onerror="alert(1)">');
    expect(container.querySelector("li strong")?.textContent).toBe("bold");
    expect(container.querySelector("li")?.textContent).toBe("bold <script>alert(1)</script>");
    expect(container.querySelector("p")?.textContent).toBe("<b>raw</b> & more");
  });

  it("shows the draft banner for a draft and hides it for a final text", () => {
    const draft = parseLegal("---\nstatus: DRAFT\n---\n# T\n");
    const { rerender } = render(<LegalView doc={draft} />);
    expect(screen.getByRole("status")).toHaveTextContent("Draft: not the final text");
    rerender(<LegalView doc={{ ...draft, draft: false }} />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("shows /terms and /privacy in the UI language, without signing in", async () => {
    mockApi({ "GET /user/get-me": signedOut });
    const { unmount } = renderApp("/terms");
    expect(await screen.findByRole("heading", { level: 1, name: "Terms of Use" })).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("Draft: not the final text");
    unmount();
    await i18n.changeLanguage("vi");
    renderApp("/privacy");
    expect(
      await screen.findByRole("heading", { level: 1, name: "Chính sách quyền riêng tư" }),
    ).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("Bản nháp: chưa phải bản chính thức");
  });

  it("links both pages from the footer of a public page", async () => {
    mockApi({ "GET /user/get-me": signedOut });
    renderApp("/login");
    const footer = await screen.findByRole("contentinfo");
    expect(within(footer).getByRole("link", { name: "Terms of Use" })).toHaveAttribute(
      "href",
      "/terms",
    );
    expect(within(footer).getByRole("link", { name: "Privacy Policy" })).toHaveAttribute(
      "href",
      "/privacy",
    );
  });
});

describe("sign-up checkbox", () => {
  it("keeps the button disabled until the box is ticked, then sends the version; the titles open the pages in a new tab", async () => {
    const calls = mockApi({ "GET /user/get-me": signedOut, "POST /auth/register": ok(user()) });
    renderApp("/register");
    const box = await screen.findByRole("checkbox", {
      name: "I agree to the Terms of Use and the Privacy Policy",
    });
    const button = screen.getByRole("button", { name: "Create account" });
    expect(button).toBeDisabled();
    const label = box.closest("label");
    for (const [name, href] of [
      ["Terms of Use", "/terms"],
      ["Privacy Policy", "/privacy"],
    ] as const) {
      const link = within(label!).getByRole("link", { name });
      expect(link).toHaveAttribute("href", href);
      expect(link).toHaveAttribute("target", "_blank");
    }
    await userEvent.click(box);
    expect(button).toBeEnabled();
    await userEvent.click(box);
    expect(button).toBeDisabled();
    await userEvent.click(box);
    await userEvent.type(screen.getByLabelText("Your name"), "Lan");
    await userEvent.type(screen.getByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "correct horse");
    await userEvent.click(button);
    await waitFor(() => expect(calls.some((c) => c.path === "/auth/register")).toBe(true));
    expect(calls.find((c) => c.path === "/auth/register")?.body).toMatchObject({
      accept_terms_version: TERMS_VERSION,
    });
  });

  it("says it in Vietnamese", async () => {
    await i18n.changeLanguage("vi");
    mockApi({ "GET /user/get-me": signedOut });
    renderApp("/register");
    expect(
      await screen.findByRole("checkbox", {
        name: "Tôi đồng ý với Điều khoản sử dụng và Chính sách quyền riêng tư",
      }),
    ).toBeInTheDocument();
  });
});

describe("accept screen", () => {
  it("sends a user who must accept there, shows both pages and returns after accepting", async () => {
    let accepted = false;
    const calls = mockApi({
      "GET /user/get-me": () => me(accepted ? ACCEPTED : OLD),
      "POST /user/accept-terms": () => {
        accepted = true;
        return me(ACCEPTED);
      },
      "GET /yearbook/get-list": () => ok({ items: [], next_id: "" }),
    });
    renderApp("/yearbooks");
    expect(await screen.findByRole("heading", { name: "Our terms have changed" })).toBeVisible();
    expect(where()).toBe("/accept-terms");
    expect(calls.some((c) => c.path.startsWith("/yearbook/"))).toBe(false);
    const accept = main().getByRole("button", { name: "Accept and continue" });
    expect(accept).toBeDisabled();
    expect(main().getByRole("link", { name: "Terms of Use" })).toHaveAttribute("href", "/terms");
    expect(main().getByRole("link", { name: "Privacy Policy" })).toHaveAttribute(
      "href",
      "/privacy",
    );
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(accept);
    expect(await screen.findByRole("heading", { name: "My yearbooks" })).toBeVisible();
    expect(where()).toBe("/yearbooks");
    expect(calls.find((c) => c.path === "/user/accept-terms")?.body).toEqual({
      version: TERMS_VERSION,
    });
  });

  it("says 'Before you start' for an account that never accepted, in both languages", async () => {
    const first = { accepted_version: null, current_version: TERMS_VERSION, required: true };
    mockApi({ "GET /user/get-me": () => me(first) });
    const { unmount } = renderApp("/account");
    expect(await screen.findByRole("heading", { name: "Before you start" })).toBeVisible();
    expect(where()).toBe("/accept-terms");
    unmount();
    await i18n.changeLanguage("vi");
    renderApp("/account");
    expect(await screen.findByRole("heading", { name: "Trước khi bắt đầu" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Đồng ý và tiếp tục" })).toBeDisabled();
    expect(main().getByRole("button", { name: "Đăng xuất" })).toBeEnabled();
    await i18n.changeLanguage("en");
    mockApi({ "GET /user/get-me": () => me(OLD) });
    renderApp("/account");
    await waitFor(() =>
      expect(
        screen.getAllByRole("heading", { name: "Our terms have changed" }).length,
      ).toBeGreaterThan(0),
    );
  });

  it("signs out from the accept screen", async () => {
    const calls = mockApi({
      "GET /user/get-me": () => me(OLD),
      "POST /auth/logout": ok(),
    });
    renderApp("/account");
    await screen.findByRole("heading", { name: "Our terms have changed" });
    await userEvent.click(main().getByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeVisible();
    expect(calls.some((c) => c.path === "/auth/logout")).toBe(true);
  });

  it("any call answering ERROR_TERMS_REQUIRED re-reads get-me and lands on the accept screen", async () => {
    let required = false;
    mockApi({
      "GET /user/get-me": () => me(required ? OLD : ACCEPTED),
      "GET /yearbook/get-list": () => {
        required = true; // the texts changed after this session started
        return err(403, "terms_required");
      },
    });
    renderApp("/yearbooks");
    expect(await screen.findByRole("heading", { name: "Our terms have changed" })).toBeVisible();
    expect(where()).toBe("/accept-terms");
  });

  it("a user who has accepted is sent on", async () => {
    mockApi({ "GET /user/get-me": () => me(ACCEPTED) });
    renderApp("/accept-terms");
    expect(await screen.findByRole("heading", { name: "My account" })).toBeVisible();
  });

  it("login re-reads get-me, so the accept screen comes straight after signing in", async () => {
    let signedIn = false;
    mockApi({
      "GET /user/get-me": () => (signedIn ? me(OLD) : signedOut),
      "POST /auth/login": () => {
        signedIn = true;
        return ok(user());
      },
    });
    renderApp("/login");
    await userEvent.type(await screen.findByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "correct horse");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("heading", { name: "Our terms have changed" })).toBeVisible();
  });
});

describe("signed-in shell", () => {
  it("links both pages", async () => {
    mockApi({
      "GET /user/get-me": () => me(ACCEPTED),
      "GET /yearbook/get-list": () => ok({ items: [], next_id: "" }),
    });
    renderApp("/yearbooks");
    await screen.findByRole("heading", { name: "My yearbooks" });
    expect(screen.getAllByRole("link", { name: "Terms of Use" })[0]).toHaveAttribute(
      "href",
      "/terms",
    );
    expect(screen.getAllByRole("link", { name: "Privacy Policy" })[0]).toHaveAttribute(
      "href",
      "/privacy",
    );
  });
});
