import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const DAY = 86_400_000;
const when = (ms: number) =>
  new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(ms);
const field = () => screen.findByLabelText("New username");
const save = () => userEvent.click(screen.getByRole("button", { name: "Save username" }));
const routes = (me: object, update?: Response | (() => Response)) => ({
  "GET /user/get-me": ok(me),
  ...(update ? { "POST /user/update-username": update } : {}),
});

afterEach(cleanup);

describe("account username", () => {
  it("shows the generated name and, after a save, the new name, the next change date and a focused confirmation", async () => {
    const nextAt = Date.now() + 7 * DAY;
    const calls = mockApi(
      routes(user(), ok(user({ username: "lan.nguyen", username_next_change_at: nextAt }))),
    );
    renderApp("/account");
    expect(await screen.findByText("user-0c1d2e3f", { selector: "p" })).toBeInTheDocument();
    const input = await field();
    expect(input).toHaveAttribute("autocomplete", "off");
    expect(input).toHaveAccessibleDescription(/3 to 30 characters/);
    await userEvent.type(input, "  Lan.Nguyen ");
    expect(input).toHaveValue("  lan.nguyen "); // lower-cased as typed
    await save();
    const note = await screen.findByRole("status");
    expect(note).toHaveTextContent(
      `Username saved. You can change your username again on ${when(nextAt)}.`,
    );
    expect(note).toHaveFocus();
    expect(screen.getByText("lan.nguyen", { selector: "p" })).toBeInTheDocument();
    expect(calls.filter((c) => c.method === "POST")[0]?.body).toEqual({ username: "lan.nguyen" });
    expect(input).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save username" })).toBeDisabled();
  });

  it("disables the form and shows the date while the cooldown runs", async () => {
    const nextAt = Date.now() + 3 * DAY;
    mockApi(routes(user({ username: "lan.nguyen", username_next_change_at: nextAt })));
    renderApp("/account");
    expect(await field()).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save username" })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(`again on ${when(nextAt)}`);
  });

  it("enables the form when the cooldown is over", async () => {
    mockApi(routes(user({ username_next_change_at: Date.now() - 1000 })));
    renderApp("/account");
    expect(await field()).toBeEnabled();
  });

  it.each([
    ["invalid_username", 400, /Use 3 to 30 characters/],
    ["username_reserved", 400, /not available/],
    ["username_taken", 409, /already taken/],
    ["rate_limited", 429, /Too many attempts. Try again in 5 min/],
  ])(
    "%s: shows its message under the field, moves focus back and does not retry",
    async (code, status, text) => {
      const calls = mockApi(
        routes(user(), err(status, code, status === 429 ? { "Retry-After": "300" } : undefined)),
      );
      renderApp("/account");
      const input = await field();
      await userEvent.type(input, "somebody");
      await save();
      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(text);
      expect(input).toHaveAccessibleDescription(expect.stringMatching(text));
      expect(input).toHaveAttribute("aria-invalid", "true");
      await waitFor(() => expect(input).toHaveFocus());
      expect(calls.filter((c) => c.method === "POST")).toHaveLength(1);
    },
  );

  it("locks the form with the date when the server says it is too soon", async () => {
    mockApi(
      routes(user(), err(429, "username_change_too_soon", { "Retry-After": String(2 * 86400) })),
    );
    renderApp("/account");
    await userEvent.type(await field(), "somebody");
    await save();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(
      /You changed your username recently. You can change it again on/,
    );
    expect(await field()).toBeDisabled();
    expect(alert).toHaveFocus();
  });

  it("hints at a bad name without calling the server and focuses the field", async () => {
    const calls = mockApi(routes(user()));
    renderApp("/account");
    const input = await field();
    await userEvent.type(input, "a b");
    await save();
    expect(await screen.findByRole("alert")).toHaveTextContent(/Use 3 to 30 characters/);
    expect(input).toHaveFocus();
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(0);
  });

  it("does not send an empty name", async () => {
    const calls = mockApi(routes(user()));
    renderApp("/account");
    await field();
    await save();
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(0);
  });

  it("sends a double submit once", async () => {
    const calls = mockApi(routes(user(), ok(user({ username: "lan.nguyen" }))));
    renderApp("/account");
    const input = await field();
    await userEvent.type(input, "lan.nguyen{Enter}{Enter}");
    await screen.findByText("Username saved.");
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(1);
  });

  it("sends the user to sign in when the session ends during the save", async () => {
    mockApi({ ...routes(user(), err(401, "unauthenticated")) });
    renderApp("/account");
    await userEvent.type(await field(), "somebody");
    await save();
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/login"));
  });
});
