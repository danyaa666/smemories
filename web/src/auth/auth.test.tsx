import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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
    });
  });
});

describe("forgot password", () => {
  it("shows the confirmation after 202", async () => {
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": Response.json({}, { status: 202 }),
    });
    renderApp("/forgot-password");
    await fill("Email", "nobody@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByRole("status")).toHaveTextContent("If an account exists");
    expect(calls.find((c) => c.path === "/v1/auth/forgot-password")?.body).toEqual({
      email: "nobody@example.com",
    });
  });

  it("maps invalid_email to the field and 429 to a wait time", async () => {
    let status = 400;
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": () =>
        status === 400
          ? err(400, "invalid_email")
          : err(429, "rate_limited", { "Retry-After": "61" }),
    });
    renderApp("/forgot-password");
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true"),
    );
    status = 429;
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByText(/Try again in 2 min/)).toBeInTheDocument();
  });
});

describe("reset password", () => {
  it("removes the token from the address bar before the request, then sets the password", async () => {
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/reset-password": new Response(null, { status: 204 }),
    });
    renderApp("/reset-password?token=abc_DEF-123", { browser: true });
    expect(window.location.search).toBe("");
    await fill("New password", "a new password");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByText(/password has been changed/)).toBeInTheDocument();
    const post = calls.find((c) => c.path === "/v1/auth/reset-password");
    expect(post?.body).toEqual({ token: "abc_DEF-123", password: "a new password" });
    expect(post?.search).toBe("");
  });

  it("keeps the form after a weak password and offers a new link after invalid_token", async () => {
    let code = "weak_password";
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/reset-password": () => err(400, code),
    });
    renderApp("/reset-password?token=t");
    await fill("New password", "short");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    await waitFor(() =>
      expect(screen.getByLabelText("New password")).toHaveAttribute("aria-invalid", "true"),
    );
    code = "invalid_token";
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("expired or was already used");
    expect(screen.getByRole("link", { name: "Ask for a new link" })).toHaveAttribute(
      "href",
      "/forgot-password",
    );
  });

  it("explains a link without a token", async () => {
    mockApi({ "GET /v1/me": signedOut });
    renderApp("/reset-password");
    expect(await screen.findByRole("alert")).toHaveTextContent("expired or was already used");
  });
});

describe("verify email", () => {
  it("sends the token once (also under StrictMode) after it left the address bar", async () => {
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/verify-email": new Response(null, { status: 204 }),
    });
    renderApp("/verify-email?token=tok_1", { browser: true, strict: true });
    expect(await screen.findByText("Your email address is verified.")).toBeInTheDocument();
    const posts = calls.filter((c) => c.path === "/v1/auth/verify-email");
    expect(posts).toHaveLength(1);
    expect(posts[0]?.body).toEqual({ token: "tok_1" });
    expect(posts[0]?.search).toBe("");
    expect(window.location.search).toBe("");
  });

  it("refreshes the session so the account shows as verified", async () => {
    let verified = false;
    mockApi({
      "GET /v1/me": () => Response.json({ user: user({ email_verified: verified }) }),
      "POST /v1/auth/verify-email": () => {
        verified = true;
        return new Response(null, { status: 204 });
      },
    });
    renderApp("/verify-email?token=t");
    await userEvent.click(await screen.findByRole("link", { name: "Continue" }));
    expect(await screen.findByText("Email verified")).toBeInTheDocument();
  });

  it.each([
    ["an invalid token", "/verify-email?token=bad", "expired or was already used"],
    ["no token", "/verify-email", "expired or was already used"],
  ])("shows the message for %s", async (_n, path, text) => {
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/verify-email": err(400, "invalid_token"),
    });
    renderApp(path);
    expect(await screen.findByRole("alert")).toHaveTextContent(text);
  });
});

describe("resend verification", () => {
  const unverified = () => Response.json({ user: user({ email_verified: false }) });

  it("confirms 202, and shows 3-per-hour and the wait on 429", async () => {
    let n = 0;
    mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email/resend": () =>
        n++ === 0
          ? Response.json({}, { status: 202 })
          : err(429, "rate_limited", { "Retry-After": "1800" }),
    });
    renderApp("/account");
    expect(await screen.findByText("You can request up to 3 emails per hour.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Send the link again" }));
    expect(await screen.findByRole("status")).toHaveTextContent("We sent a new link.");
    await userEvent.click(screen.getByRole("button", { name: "Send the link again" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Try again in 30 min.");
  });

  it("says so when the address was already verified", async () => {
    mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email/resend": Response.json({ already_verified: true }),
    });
    renderApp("/account");
    await userEvent.click(await screen.findByRole("button", { name: "Send the link again" }));
    expect(await screen.findByRole("status")).toHaveTextContent("already verified");
  });
});
