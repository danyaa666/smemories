import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import i18n from "../i18n";
import { type Call, err, mockApi, renderApp, user } from "../test/api";

const signedOut = err(401, "unauthenticated");
const unverified = () => Response.json({ user: user({ email_verified: false }) });
const where = () => screen.getByTestId("where").textContent;
const ok204 = () => new Response(null, { status: 204 });
const verifyCalls = (calls: Call[]) => calls.filter((c) => c.path === "/v1/auth/verify-email");

afterEach(() => vi.useRealTimers());

async function codeInput() {
  return screen.findByLabelText("6-digit code");
}

describe("verify email", () => {
  it("shows the address, a numeric one-time-code field and submits on the sixth digit", async () => {
    const calls = mockApi({ "GET /v1/me": unverified, "POST /v1/auth/verify-email": ok204 });
    renderApp("/verify-email");
    expect(await screen.findByText(/sent a 6-digit code to lan@example.com/)).toBeInTheDocument();
    const input = await codeInput();
    expect(input).toHaveAttribute("inputmode", "numeric");
    expect(input).toHaveAttribute("autocomplete", "one-time-code");
    expect(input).toHaveAttribute("maxlength", "6");
    await userEvent.type(input, "12345");
    expect(verifyCalls(calls)).toHaveLength(0);
    await userEvent.type(input, "6");
    expect(await screen.findByText("Your email address is verified.")).toBeInTheDocument();
    expect(verifyCalls(calls)).toHaveLength(1);
    expect(verifyCalls(calls)[0]?.body).toEqual({ code: "123456" });
  });

  it("accepts a pasted code with spaces and a trailing newline, and drops letters", async () => {
    const calls = mockApi({ "GET /v1/me": unverified, "POST /v1/auth/verify-email": ok204 });
    renderApp("/verify-email");
    const input = await codeInput();
    await userEvent.click(input);
    await userEvent.paste("123 456\n");
    await waitFor(() => expect(verifyCalls(calls)).toHaveLength(1));
    expect(verifyCalls(calls)[0]?.body).toEqual({ code: "123456" });
  });

  it("keeps digits only when typing letters", async () => {
    mockApi({ "GET /v1/me": unverified });
    renderApp("/verify-email");
    const input = await codeInput();
    await userEvent.type(input, "a1b2-3");
    expect(input).toHaveValue("123");
  });

  it("submits a short code on Enter without a request and says it is wrong", async () => {
    const calls = mockApi({ "GET /v1/me": unverified });
    renderApp("/verify-email");
    await userEvent.type(await codeInput(), "123{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("That code is wrong");
    expect(verifyCalls(calls)).toHaveLength(0);
  });

  it("refreshes the session so the account shows as verified", async () => {
    let verified = false;
    mockApi({
      "GET /v1/me": () => Response.json({ user: user({ email_verified: verified }) }),
      "POST /v1/auth/verify-email": () => {
        verified = true;
        return ok204();
      },
    });
    renderApp("/verify-email");
    await userEvent.type(await codeInput(), "123456");
    await userEvent.click(await screen.findByRole("link", { name: "Continue" }));
    expect(await screen.findByText("Email verified")).toBeInTheDocument();
  });

  it("sends a signed-out visitor to sign in", async () => {
    mockApi({ "GET /v1/me": signedOut });
    renderApp("/verify-email");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
  });

  it.each([
    ["invalid_code", 400, "That code is wrong"],
    ["code_expired", 400, "has expired. Send a new code"],
    ["code_locked", 400, "Too many wrong attempts"],
    ["code_store_unavailable", 503, "cannot check codes right now"],
    ["session_store_unavailable", 503, "cannot reach the sign-in service"],
  ])("maps %s next to the field or above the button", async (code, status, text) => {
    mockApi({ "GET /v1/me": unverified, "POST /v1/auth/verify-email": err(status, code) });
    renderApp("/verify-email");
    const input = await codeInput();
    await userEvent.type(input, "000000");
    expect(await screen.findByRole("alert")).toHaveTextContent(text);
    expect(screen.getByRole("alert")).not.toHaveTextContent("Something went wrong");
    if (code.endsWith("_code") || code.startsWith("code_e") || code === "code_locked") {
      expect(input).toHaveAttribute("aria-invalid", "true");
      expect(input).toHaveFocus();
    }
  });

  it("explains rate_limited with the Retry-After time, and a network error", async () => {
    let n = 0;
    mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email": () => {
        if (n++ === 0) return err(429, "rate_limited", { "Retry-After": "1800" });
        throw new TypeError("offline");
      },
    });
    renderApp("/verify-email");
    const input = await codeInput();
    await userEvent.type(input, "111111");
    expect(await screen.findByRole("alert")).toHaveTextContent("Try again in 30 min.");
    await userEvent.type(input, "{Backspace}1");
    expect(await screen.findByText(/Could not reach the server/)).toBeInTheDocument();
  });

  it("sends once for Enter pressed right after the auto-submit", async () => {
    let resolve: (r: Response) => void = () => undefined;
    const calls = mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email": () => {
        return new Promise<Response>((r) => (resolve = r)) as unknown as Response;
      },
    });
    renderApp("/verify-email");
    const input = await codeInput();
    await userEvent.type(input, "123456{Enter}{Enter}");
    expect(verifyCalls(calls)).toHaveLength(1);
    resolve(ok204());
  });

  it("is in Vietnamese too, and never keeps the code in the address or storage", async () => {
    await i18n.changeLanguage("vi");
    mockApi({ "GET /v1/me": unverified, "POST /v1/auth/verify-email": err(400, "invalid_code") });
    renderApp("/verify-email", { browser: true });
    await userEvent.type(await screen.findByLabelText("Mã gồm 6 chữ số"), "654321");
    expect(await screen.findByRole("alert")).toHaveTextContent("Mã không đúng");
    expect(window.location.href).not.toContain("654321");
    expect(JSON.stringify({ ...localStorage, ...sessionStorage })).not.toContain("654321");
  });
});

describe("resend the verification code", () => {
  it("counts down 60 seconds after each send and enables the button again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const calls = mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email/resend": Response.json({}, { status: 202 }),
    });
    renderApp("/verify-email");
    const btn = await screen.findByRole("button", { name: "Send a new code" });
    expect(btn).toBeEnabled();
    fireEvent.click(btn);
    expect(await screen.findByRole("status")).toHaveTextContent("We sent a new code.");
    expect(screen.getByRole("button", { name: /Send a new code \(\d+s\)/ })).toBeDisabled();
    await act(() => vi.advanceTimersByTimeAsync(30_000));
    expect(screen.getByRole("button", { name: /Send a new code \((2|3)\ds\)/ })).toBeDisabled();
    await act(() => vi.advanceTimersByTimeAsync(31_000));
    expect(screen.getByRole("button", { name: "Send a new code" })).toBeEnabled();
    expect(calls.filter((c) => c.path.endsWith("/resend"))).toHaveLength(1);
  });

  it("starts the cooldown when the student just registered", async () => {
    let signedIn = false;
    mockApi({
      "GET /v1/me": () => (signedIn ? unverified() : signedOut),
      "POST /v1/auth/register": () => {
        signedIn = true;
        return unverified();
      },
    });
    renderApp("/register");
    await userEvent.type(await screen.findByLabelText("Your name"), "Lan");
    await userEvent.type(screen.getByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "correct horse");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByText(/sent a 6-digit code to lan@example.com/)).toBeInTheDocument();
    expect(where()).toBe("/verify-email");
    expect(screen.getByRole("button", { name: "Send a new code (60s)" })).toBeDisabled();
  });

  it("shows the 429 message when the API refuses and clears the old wrong-code error on success", async () => {
    let n = 0;
    mockApi({
      "GET /v1/me": unverified,
      "POST /v1/auth/verify-email": err(400, "code_expired"),
      "POST /v1/auth/verify-email/resend": () =>
        n++ === 0
          ? err(429, "rate_limited", { "Retry-After": "3000" })
          : Response.json({}, { status: 202 }),
    });
    renderApp("/verify-email");
    await userEvent.type(await codeInput(), "123456");
    expect(await screen.findByRole("alert")).toHaveTextContent("has expired");
    await userEvent.click(screen.getByRole("button", { name: "Send a new code" }));
    expect(await screen.findByText(/Try again in 50 min\./)).toBeInTheDocument();
    // a refused send starts no cooldown
    await userEvent.click(screen.getByRole("button", { name: "Send a new code" }));
    expect(await screen.findByRole("status")).toHaveTextContent("We sent a new code.");
    expect(screen.queryByText(/has expired/)).not.toBeInTheDocument();
    expect(await codeInput()).toHaveValue("");
  });

  it("says so when the address was already verified", async () => {
    let verified = false;
    mockApi({
      "GET /v1/me": () => Response.json({ user: user({ email_verified: verified }) }),
      "POST /v1/auth/verify-email/resend": () => {
        verified = true;
        return Response.json({ already_verified: true });
      },
    });
    renderApp("/verify-email");
    await userEvent.click(await screen.findByRole("button", { name: "Send a new code" }));
    expect(await screen.findByText("Your email address is verified.")).toBeInTheDocument();
  });
});

describe("account page", () => {
  it("links an unverified student to the code screen", async () => {
    mockApi({ "GET /v1/me": unverified });
    renderApp("/account");
    await userEvent.click(await screen.findByRole("link", { name: "Verify your email" }));
    expect(where()).toBe("/verify-email");
  });
});

describe("forgot and reset password", () => {
  it("asks for the email, then moves to the reset screen with a neutral message", async () => {
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": Response.json({}, { status: 202 }),
    });
    renderApp("/forgot-password");
    await userEvent.type(await screen.findByLabelText("Email"), "nobody@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send code" }));
    expect(await screen.findByRole("status")).toHaveTextContent("If an account exists");
    expect(where()).toBe("/reset-password");
    expect(screen.getByLabelText("Email")).toHaveValue("nobody@example.com");
    expect(screen.getByLabelText("6-digit code")).toHaveFocus();
    expect(screen.getByLabelText("New password")).toHaveAccessibleDescription(/10 characters/);
    expect(calls.find((c) => c.path === "/v1/auth/forgot-password")?.body).toEqual({
      email: "nobody@example.com",
    });
  });

  it("maps invalid_email on the first step and stays there", async () => {
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": err(400, "invalid_email"),
    });
    renderApp("/forgot-password");
    await userEvent.click(await screen.findByRole("button", { name: "Send code" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true"),
    );
    expect(where()).toBe("/forgot-password");
  });

  it("changes the password with email, code and password, then shows the success on the login page", async () => {
    const calls = mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/reset-password": ok204 });
    renderApp("/reset-password");
    await userEvent.type(await screen.findByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("6-digit code"), "123 456");
    await userEvent.type(screen.getByLabelText("New password"), "a new password");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("password has been changed");
    expect(calls.find((c) => c.path === "/v1/auth/reset-password")?.body).toEqual({
      email: "lan@example.com",
      code: "123456",
      password: "a new password",
    });
  });

  it.each([
    ["invalid_code", 400, "That code is wrong", "6-digit code"],
    ["code_expired", 400, "has expired", "6-digit code"],
    ["code_locked", 400, "Too many wrong attempts", "6-digit code"],
    ["weak_password", 400, "10 to 128 characters", "New password"],
    ["invalid_email", 400, "valid email", "Email"],
    ["code_store_unavailable", 503, "cannot check codes", ""],
    ["busy", 503, "code has been used up", ""],
  ])("maps %s", async (code, status, text, field) => {
    mockApi({ "GET /v1/me": signedOut, "POST /v1/auth/reset-password": err(status, code) });
    renderApp("/reset-password");
    await userEvent.type(await screen.findByLabelText("Email"), "lan@example.com");
    await userEvent.type(screen.getByLabelText("6-digit code"), "123456");
    await userEvent.type(screen.getByLabelText("New password"), "a new password");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    const alert = await screen.findByText(new RegExp(text));
    expect(alert).toBeInTheDocument();
    if (field) expect(screen.getByLabelText(field)).toHaveAttribute("aria-invalid", "true");
    expect(where()).toBe("/reset-password");
  });

  it("does not send a code that is not 6 digits", async () => {
    const calls = mockApi({ "GET /v1/me": signedOut });
    renderApp("/reset-password");
    await userEvent.type(await screen.findByLabelText("6-digit code"), "12");
    await userEvent.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByText(/That code is wrong/)).toBeInTheDocument();
    expect(calls.filter((c) => c.path === "/v1/auth/reset-password")).toHaveLength(0);
  });

  it("resends with the email typed, with the same cooldown", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const calls = mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": Response.json({}, { status: 202 }),
    });
    renderApp("/reset-password");
    fireEvent.change(await screen.findByLabelText("Email"), { target: { value: "a@b.co" } });
    fireEvent.click(screen.getByRole("button", { name: "Send a new code" }));
    expect(await screen.findByRole("button", { name: "Send a new code (60s)" })).toBeDisabled();
    expect(calls.find((c) => c.path === "/v1/auth/forgot-password")?.body).toEqual({
      email: "a@b.co",
    });
    await act(() => vi.advanceTimersByTimeAsync(61_000));
    expect(screen.getByRole("button", { name: "Send a new code" })).toBeEnabled();
  });

  it("shows the 429 of the resend", async () => {
    mockApi({
      "GET /v1/me": signedOut,
      "POST /v1/auth/forgot-password": err(429, "rate_limited", { "Retry-After": "600" }),
    });
    renderApp("/reset-password");
    await userEvent.type(await screen.findByLabelText("Email"), "a@b.co");
    await userEvent.click(screen.getByRole("button", { name: "Send a new code" }));
    expect(await screen.findByText(/Try again in 10 min\./)).toBeInTheDocument();
  });
});

describe("old link pages", () => {
  it("do not read a token from the address", async () => {
    const calls = mockApi({ "GET /v1/me": signedOut });
    renderApp("/reset-password?token=secret_tok", { browser: true });
    await screen.findByLabelText("6-digit code");
    expect(calls.every((c) => c.body === undefined)).toBe(true);
  });
});
