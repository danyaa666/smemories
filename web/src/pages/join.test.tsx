import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const CID = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0BKBK";
const TOKEN = "Zq3-Tk9_wX1mN7vB5cL2hD8fR4sG6jP0aE1yU3iO9tA";
const main = () => within(screen.getByRole("main"));
const where = () => screen.getByTestId("where").textContent;
const signedOut = err(401, "unauthenticated");
const preview = ok({ name: "Lớp 12A1", school_name: "THPT Chu Văn An" });
const joined = ok({
  id: CID,
  name: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  graduation_year: 2026,
  language: "vi",
  my_role: "student",
  page_linked: false,
  created_at: 1760000000000,
  updated_at: 1760000000000,
});
const base = { "POST /class-invite/lookup": preview };

describe("join page", () => {
  it("previews the class for a signed-out visitor and offers sign-up and sign-in", async () => {
    const calls = mockApi({ ...base, "GET /user/get-me": signedOut });
    renderApp(`/join/${TOKEN}`);
    expect(await screen.findByRole("heading", { name: "Join Lớp 12A1" })).toBeInTheDocument();
    expect(screen.getByText("THPT Chu Văn An")).toBeInTheDocument();
    expect(main().getByRole("link", { name: "Create account" })).toHaveAttribute(
      "href",
      "/register",
    );
    expect(main().getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
    expect(screen.queryByRole("button", { name: "Join this class" })).not.toBeInTheDocument();
    const lookup = calls.find((c) => c.path === "/class-invite/lookup")!;
    expect(lookup.method).toBe("POST");
    expect(lookup.body).toEqual({ token: TOKEN });
  });

  it("shows one neutral message for every dead link, and nothing about why", async () => {
    const texts: string[] = [];
    for (const message of ["unknown", "expired", "revoked"]) {
      mockApi({
        "GET /user/get-me": signedOut,
        "POST /class-invite/lookup": Response.json(
          { status: "ERROR_NOT_FOUND", error_message: message, request_id: "r1" },
          { status: 404 },
        ),
      });
      const { container, unmount } = renderApp(`/join/${TOKEN}`);
      expect(await screen.findByRole("alert")).toHaveTextContent("This invite is not valid.");
      texts.push(container.querySelector("main")?.textContent ?? "");
      unmount();
    }
    expect(new Set(texts).size).toBe(1);
    expect(texts[0]).not.toMatch(/expired|revoked|unknown/i);
    expect(texts[0]).not.toContain(TOKEN);
  });

  it("offers a retry, not the neutral message, when the server cannot be reached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    renderApp(`/join/${TOKEN}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not reach the server");
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("joins when signed in, then creates my page prefilled and opens the editor", async () => {
    const calls = mockApi({
      ...base,
      "GET /user/get-me": ok(user()),
      "POST /class-invite/accept": joined,
      "POST /yearbook/create": ok({ id: BOOK }),
      "POST /class-member/set-book": ok({
        linked: true,
        yearbook_id: BOOK,
        profile_complete: false,
        approved_notes: 0,
      }),
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await screen.findByRole("button", { name: "Join this class" }));
    expect(await screen.findByRole("heading", { name: "You joined Lớp 12A1" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to the class" })).toHaveAttribute(
      "href",
      `/classes/${CID}`,
    );
    await userEvent.click(screen.getByRole("button", { name: "Create my page" }));
    await waitFor(() => expect(where()).toBe(`/yearbooks/${BOOK}`));
    expect(calls.find((c) => c.path === "/class-invite/accept")?.body).toEqual({ token: TOKEN });
    expect(calls.find((c) => c.path === "/yearbook/create")?.body).toMatchObject({
      title: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      graduation_year: 2026,
      language: "vi",
    });
    expect(calls.find((c) => c.path === "/class-member/set-book")?.body).toEqual({
      class_id: CID,
      yearbook_id: BOOK,
    });
  });

  it("answers a link that died between preview and accept with the same neutral message", async () => {
    mockApi({
      ...base,
      "GET /user/get-me": ok(user()),
      "POST /class-invite/accept": err(404, "not_found"),
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await screen.findByRole("button", { name: "Join this class" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This invite is not valid.");
  });

  it("says when the class is full", async () => {
    mockApi({
      ...base,
      "GET /user/get-me": ok(user()),
      "POST /class-invite/accept": err(409, "class_full"),
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await screen.findByRole("button", { name: "Join this class" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This class is full");
  });

  it("sends an unverified email to the code screen and back here afterwards", async () => {
    mockApi({ ...base, "GET /user/get-me": ok(user({ email_verified: false })) });
    renderApp(`/join/${TOKEN}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("Verify your email");
    expect(screen.queryByRole("button", { name: "Join this class" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("link", { name: "Verify your email" }));
    expect(where()).toBe("/verify-email");
  });

  it("honours the Terms gate", async () => {
    mockApi({
      ...base,
      "GET /user/get-me": ok(user({ terms: { required: true, accepted_version: "" } })),
    });
    renderApp(`/join/${TOKEN}`);
    await waitFor(() => expect(where()).toBe("/accept-terms"));
  });

  it("returns here after sign-up and then joins", async () => {
    let signedIn = false;
    const calls = mockApi({
      ...base,
      "GET /user/get-me": () => (signedIn ? ok(user()) : signedOut),
      "POST /auth/register": () => {
        signedIn = true;
        return ok(user());
      },
      "POST /class-invite/accept": joined,
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await main().findByRole("link", { name: "Create account" }));
    expect(where()).toBe("/register");
    // Google sign-in would put the join path into a query string
    expect(screen.queryByText(/Google/)).not.toBeInTheDocument();
    await userEvent.type(await screen.findByLabelText("Your name"), "Lan");
    await userEvent.type(screen.getByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText(/^Password/), "correct horse battery");
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    await waitFor(() => expect(where()).toBe(`/join/${TOKEN}`));
    await userEvent.click(await screen.findByRole("button", { name: "Join this class" }));
    expect(await screen.findByRole("heading", { name: "You joined Lớp 12A1" })).toBeInTheDocument();
    expect(calls.some((c) => c.path === "/class-invite/accept")).toBe(true);
  });

  it("after sign-up with an unverified email, the code screen continues to the invite", async () => {
    let state: "out" | "new" | "verified" = "out";
    mockApi({
      ...base,
      "GET /user/get-me": () =>
        state === "out" ? signedOut : ok(user({ email_verified: state === "verified" })),
      "POST /auth/register": () => {
        state = "new";
        return ok(user({ email_verified: false }));
      },
      "POST /auth/verify-email": () => {
        state = "verified";
        return ok();
      },
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await main().findByRole("link", { name: "Create account" }));
    await userEvent.type(await screen.findByLabelText("Your name"), "Lan");
    await userEvent.type(screen.getByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText(/^Password/), "correct horse battery");
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    await waitFor(() => expect(where()).toBe("/verify-email"));
    await userEvent.type(await screen.findByLabelText(/code/i), "123456");
    expect(await screen.findByRole("link", { name: "Continue" })).toHaveAttribute(
      "href",
      `/join/${TOKEN}`,
    );
  });

  it("returns here after sign-in", async () => {
    let signedIn = false;
    mockApi({
      ...base,
      "GET /user/get-me": () => (signedIn ? ok(user()) : signedOut),
      "POST /auth/login": () => {
        signedIn = true;
        return ok(user());
      },
    });
    renderApp(`/join/${TOKEN}`);
    await userEvent.click(await main().findByRole("link", { name: "Sign in" }));
    expect(screen.queryByText(/Google/)).not.toBeInTheDocument();
    await userEvent.type(await screen.findByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "correct horse battery");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(where()).toBe(`/join/${TOKEN}`));
    expect(await screen.findByRole("button", { name: "Join this class" })).toBeInTheDocument();
  });

  it("keeps the token out of the query string, storage, the console and every URL", async () => {
    const spies = (["log", "info", "warn", "error", "debug"] as const).map((m) =>
      vi.spyOn(console, m),
    );
    const calls = mockApi({
      ...base,
      "GET /user/get-me": ok(user()),
      "POST /class-invite/accept": err(404, "not_found"),
    });
    renderApp(`/join/${TOKEN}`, { browser: true });
    await userEvent.click(await screen.findByRole("button", { name: "Join this class" }));
    await screen.findByRole("alert");
    expect(window.location.pathname).toBe(`/join/${TOKEN}`);
    expect(window.location.search).toBe("");
    expect(window.location.hash).toBe("");
    expect(JSON.stringify([{ ...localStorage }, { ...sessionStorage }])).not.toContain(TOKEN);
    expect(calls.map((c) => c.path).join("\n")).not.toContain(TOKEN);
    expect(calls.every((c) => c.search === "")).toBe(true);
    expect(calls.filter((c) => c.method === "GET" && c.path.includes("invite"))).toEqual([]);
    for (const s of spies) expect(JSON.stringify(s.mock.calls)).not.toContain(TOKEN);
  });

  it("is available in Vietnamese", async () => {
    await i18n.changeLanguage("vi");
    mockApi({ ...base, "GET /user/get-me": signedOut });
    renderApp(`/join/${TOKEN}`);
    expect(await screen.findByRole("heading", { name: "Tham gia Lớp 12A1" })).toBeInTheDocument();
    expect(screen.getByText("Hãy tạo tài khoản hoặc đăng nhập để tham gia.")).toBeInTheDocument();
  });
});
