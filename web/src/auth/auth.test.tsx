import { onlineManager } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import i18n from "../i18n";
import { err, mockApi, renderApp, user } from "../test/api";
import { safePath } from "./errors";

const signedOut = err(401, "unauthenticated");
const me = () => Response.json({ user: user() });
const where = () => screen.getByTestId("where").textContent;

async function fill(label: string, value: string) {
  await userEvent.type(await screen.findByLabelText(label), value);
}

describe("session bootstrap and protected routes", () => {
  it("sends a signed-out visitor from /account to /login", async () => {
    mockApi({ "GET /v1/me": signedOut });
    renderApp("/account");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(where()).toBe("/login");
    expect(screen.getByRole("link", { name: "Create account" })).toBeInTheDocument();
  });

  it("shows the account page and a sign-out button for a signed-in user", async () => {
    mockApi({ "GET /v1/me": me });
    renderApp("/account");
    expect(await screen.findByRole("heading", { name: "My account" })).toBeInTheDocument();
    expect(screen.getByText("Email verified")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });

  it("says so when the API cannot be reached, with a retry", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    renderApp("/account");
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not reach the server");
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("signs out: POST, then the login page", async () => {
    const calls = mockApi({
      "GET /v1/me": me,
      "POST /v1/auth/logout": new Response(null, { status: 204 }),
    });
    renderApp("/account");
    await userEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(calls.some((c) => c.method === "POST" && c.path === "/v1/auth/logout")).toBe(true);
    expect(screen.getByRole("link", { name: "Sign in" })).toBeInTheDocument();
  });

  it("only follows same-site return paths", () => {
    expect(safePath("/yearbooks/1?x=2")).toBe("/yearbooks/1?x=2");
    for (const bad of [
      "//evil.example",
      "/\\evil.example",
      "https://evil.example",
      "x",
      undefined,
    ]) {
      expect(safePath(bad)).toBe("/account");
    }
  });
});

describe("login", () => {
  it("posts JSON and lands on the page the user came from", async () => {
    let signedIn = false;
    const calls = mockApi({
      "GET /v1/me": () => (signedIn ? me() : signedOut),
      "POST /v1/auth/login": () => {
        signedIn = true;
        return me();
      },
    });
    renderApp("/account");
    await fill("Email", "lan@example.com");
    await fill("Password", "correct horse");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("heading", { name: "My account" })).toBeInTheDocument();
    expect(calls.find((c) => c.path === "/v1/auth/login")?.body).toEqual({
      email: "lan@example.com",
      password: "correct horse",
    });
  });

  it("shows the translated error for a wrong password, in Vietnamese too", async () => {
    mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/login": err(401, "invalid_credentials") });
    renderApp("/login");
    await fill("Email", "a@b.co");
    await fill("Password", "x");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Wrong email or password.");
    await i18n.changeLanguage("vi");
    expect(await screen.findByRole("alert")).toHaveTextContent("Email hoặc mật khẩu không đúng.");
  });

  it("turns 429 Retry-After into minutes", async () => {
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/login": err(429, "rate_limited", { "Retry-After": "900" }),
    });
    renderApp("/login");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Try again in 15 min.");
  });

  it("shows a generic message for an unmapped error code", async () => {
    mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/login": err(500, "internal_error") });
    renderApp("/login");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Something went wrong");
  });

  it.each([
    ["oidc_state", "took too long"],
    ["oidc_denied", "cancelled"],
    ["oidc_failed", "failed"],
    ["email_unverified", "could not confirm your email"],
    ["whatever", "Sign-in failed"],
  ])("explains ?error=%s", async (code, text) => {
    mockApi({ "GET /v1/me": signedOut });
    renderApp(`/login?error=${code}`);
    expect(await screen.findByRole("alert")).toHaveTextContent(text);
  });

  it("offers Google only when the build enables it, as a plain link with return_to", async () => {
    mockApi({ "GET /v1/me": signedOut });
    const { unmount } = renderApp("/login");
    await screen.findByRole("heading", { name: "Sign in" });
    expect(screen.queryByRole("link", { name: "Sign in with Google" })).not.toBeInTheDocument();
    unmount();
    vi.stubEnv("VITE_GOOGLE_SIGNIN", "true");
    renderApp("/login");
    expect(await screen.findByRole("link", { name: "Sign in with Google" })).toHaveAttribute(
      "href",
      "/api/v1/auth/google/start?return_to=%2Faccount",
    );
  });
});

describe("register", () => {
  it("attaches field errors to their inputs", async () => {
    mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/register": err(400, "weak_password") });
    renderApp("/register");
    await fill("Your name", "Lan");
    await fill("Email", "lan@example.com");
    await fill("Password", "short");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    const pw = await screen.findByLabelText("Password");
    await waitFor(() => expect(pw).toHaveAttribute("aria-invalid", "true"));
    expect(pw).toHaveAccessibleDescription(/At least 10 characters.*10 to 128 characters/);
    expect(screen.getByLabelText("Email")).not.toHaveAttribute("aria-invalid");
  });

  it("shows email_taken above the button and creates the account on success", async () => {
    let taken = true;
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/register": () => {
        if (taken) return err(409, "email_taken");
        return me();
      },
    });
    renderApp("/register");
    await fill("Your name", "Lan Nguyễn");
    await fill("Email", "lan@example.com");
    await fill("Password", "correct horse");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("already registered");
    taken = false;
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByRole("heading", { name: "My account" })).toBeInTheDocument();
    expect(calls.filter((c) => c.path === "/v1/auth/register").at(-1)?.body).toEqual({
      email: "lan@example.com",
      password: "correct horse",
      display_name: "Lan Nguyễn",
      locale: "en",
    });
  });

  it("sends the active UI language as locale", async () => {
    await i18n.changeLanguage("vi");
    const calls = mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/register": me });
    renderApp("/register");
    await userEvent.type(await screen.findByLabelText("Tên của bạn"), "Lan");
    await userEvent.type(screen.getByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("Mật khẩu"), "correct horse");
    await userEvent.click(screen.getByRole("button", { name: "Tạo tài khoản" }));
    await waitFor(() => expect(calls.some((c) => c.path === "/v1/auth/register")).toBe(true));
    expect(calls.find((c) => c.path === "/v1/auth/register")?.body).toMatchObject({ locale: "vi" });
  });
});

describe("offline", () => {
  it("shows the network error at once instead of waiting for the network", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    onlineManager.setOnline(false);
    onTestFinished(() => onlineManager.setOnline(true));
    renderApp("/login");
    await fill("Email", "a@b.co");
    await fill("Password", "pw");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByText(/Could not reach the server/)).toBeInTheDocument();
  });
});
