import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ClassInvite, ClassMember, SchoolClass } from "../api/classes";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const CID = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0BKBK";
const TOKEN = "Zq3-Tk9_wX1mN7vB5cL2hD8fR4sG6jP0aE1yU3iO9tA";
const LINK = `https://smemories.example/join/${TOKEN}`;
const where = () => screen.getByTestId("where").textContent;

const cls = (over: Partial<SchoolClass> = {}): SchoolClass => ({
  id: CID,
  name: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  graduation_year: 2026,
  language: "vi",
  my_role: "student",
  page_linked: false,
  created_at: 1760000000000,
  updated_at: 1760000000000,
  ...over,
});
const member = (over: Partial<ClassMember> = {}): ClassMember => ({
  id: "01J9Z3K6V8Q4M7N2P5R8T0MMM1",
  name: "Minh Trần",
  role: "student",
  joined_at: 1760000000000,
  page_linked: true,
  profile_complete: false,
  approved_notes: 3,
  ...over,
});
const invite = (over: Partial<ClassInvite> = {}): ClassInvite => ({
  id: "01J9Z3K6V8Q4M7N2P5R8T0III1",
  created_at: 1760000000000,
  expires_at: 1761000000000,
  revoked_at: null,
  active: true,
  ...over,
});
const me = () => ok(user());
const list = (items: object[], next = "") => ok({ items, next_id: next });

describe("class list", () => {
  it("shows an empty state with the create button", async () => {
    mockApi({ "GET /user/get-me": me, "GET /class/get-list": list([]) });
    renderApp("/classes");
    expect(await screen.findByText(/not in any class yet/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New class" })).toHaveAttribute("href", "/classes/new");
  });

  it("lists classes with the role as text, and pages", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      "GET /class/get-list": list([cls({ my_role: "admin" })], "c2"),
      "GET /class/get-list?next_id=c2": list([cls({ id: "X".repeat(26), name: "Second" })]),
    });
    renderApp("/classes");
    expect(await screen.findByRole("link", { name: "Lớp 12A1" })).toHaveAttribute(
      "href",
      `/classes/${CID}`,
    );
    expect(screen.getByText("Role: Admin")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByRole("link", { name: "Second" })).toBeInTheDocument();
    expect(screen.getByText("Role: Student")).toBeInTheDocument();
    expect(calls.at(-1)?.path).toBe("/class/get-list?next_id=c2");
  });

  it("shows an error with a retry", async () => {
    mockApi({ "GET /user/get-me": me, "GET /class/get-list": err(500, "internal_error") });
    renderApp("/classes");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("is reachable from the navigation", async () => {
    mockApi({ "GET /user/get-me": me, "GET /class/get-list": list([]) });
    renderApp("/classes");
    expect((await screen.findAllByRole("link", { name: /^Classes$/ })).length).toBeGreaterThan(0);
  });
});

describe("new class", () => {
  it("creates a class and opens it", async () => {
    const calls = mockApi({
      "GET /user/get-me": me,
      "POST /class/create": ok(cls({ my_role: "admin" })),
      [`GET /class/get?id=${CID}`]: ok(cls({ my_role: "admin" })),
      [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
      [`GET /class-member/get-list?class_id=${CID}`]: list([]),
    });
    renderApp("/classes/new");
    await userEvent.type(await screen.findByLabelText("Class name"), "Lớp 12A1");
    await userEvent.type(screen.getByLabelText("School"), "THPT Chu Văn An");
    await userEvent.type(screen.getByLabelText("Graduation year"), "2026");
    await userEvent.selectOptions(screen.getByLabelText("Language of the class book"), "vi");
    await userEvent.click(screen.getByRole("button", { name: "Create class" }));
    await waitFor(() => expect(where()).toBe(`/classes/${CID}`));
    expect(calls.find((c) => c.path === "/class/create")?.body).toEqual({
      name: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      graduation_year: 2026,
      language: "vi",
    });
  });

  it("puts a field error on its input and the 10-class limit above the button", async () => {
    let n = 0;
    mockApi({
      "GET /user/get-me": me,
      "POST /class/create": () =>
        n++ === 0 ? err(400, "invalid_name") : err(409, "limit_reached"),
    });
    renderApp("/classes/new");
    await userEvent.click(await screen.findByRole("button", { name: "Create class" }));
    expect(await screen.findByText(/class name of 1 to 120/)).toBeInTheDocument();
    expect(screen.getByLabelText("Class name")).toHaveAccessibleDescription(
      /class name of 1 to 120/,
    );
    await userEvent.click(screen.getByRole("button", { name: "Create class" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("admin of at most 10 classes");
  });

  it("sends an unverified email to the code screen", async () => {
    mockApi({ "GET /user/get-me": me, "POST /class/create": err(403, "email_not_verified") });
    renderApp("/classes/new");
    await userEvent.click(await screen.findByRole("button", { name: "Create class" }));
    expect(await screen.findByRole("link", { name: "Verify your email" })).toHaveAttribute(
      "href",
      "/verify-email",
    );
  });

  it("asks for the language in Vietnamese too", async () => {
    await i18n.changeLanguage("vi");
    mockApi({ "GET /user/get-me": me });
    renderApp("/classes/new");
    expect(await screen.findByRole("heading", { name: "Lớp mới" })).toBeInTheDocument();
    expect(screen.getByLabelText("Ngôn ngữ của kỷ yếu lớp")).toHaveValue("vi");
  });
});

describe("class screen, student", () => {
  const student = (over: Partial<SchoolClass> = {}) => ({
    "GET /user/get-me": me,
    [`GET /class/get?id=${CID}`]: ok(cls(over)),
  });

  it("shows no roster, no invites and no member actions, and makes no such request", async () => {
    const calls = mockApi(student());
    renderApp(`/classes/${CID}`);
    expect(await screen.findByRole("heading", { name: "Lớp 12A1" })).toBeInTheDocument();
    expect(screen.getByText("Student")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Members" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Invite links" })).not.toBeInTheDocument();
    for (const name of [/Create invite link/, /Remove/, /Make .* admin/, /Revoke/]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(calls.filter((c) => /class-member\/get-list|class-invite/.test(c.path))).toEqual([]);
    expect(calls.map((c) => c.path).sort()).toEqual(
      ["/user/get-me", `/class/get?id=${CID}`].sort(),
    );
  });

  it("offers to create my page, prefilled from the class, then opens the editor", async () => {
    const calls = mockApi({
      ...student(),
      "POST /yearbook/create": ok({ id: BOOK }),
      "POST /class-member/set-book": ok({
        linked: true,
        yearbook_id: BOOK,
        profile_complete: false,
        approved_notes: 0,
      }),
    });
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Create my page" }));
    await waitFor(() => expect(where()).toBe(`/yearbooks/${BOOK}`));
    expect(calls.find((c) => c.path === "/yearbook/create")?.body).toEqual({
      title: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      class_name: "Lớp 12A1",
      graduation_year: 2026,
      motto: "",
      language: "vi",
      page_size: "A5",
    });
    expect(calls.find((c) => c.path === "/class-member/set-book")?.body).toEqual({
      class_id: CID,
      yearbook_id: BOOK,
    });
  });

  it("when linking fails, a retry links the same book and does not create a second one", async () => {
    let n = 0;
    const calls = mockApi({
      ...student(),
      "POST /yearbook/create": ok({ id: BOOK }),
      "POST /class-member/set-book": () =>
        n++ === 0
          ? err(409, "conflict")
          : ok({ linked: true, yearbook_id: BOOK, profile_complete: false, approved_notes: 0 }),
    });
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Create my page" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("already the page of another class");
    await userEvent.click(screen.getByRole("button", { name: "Create my page" }));
    await waitFor(() => expect(where()).toBe(`/yearbooks/${BOOK}`));
    expect(calls.filter((c) => c.path === "/yearbook/create")).toHaveLength(1);
  });

  it("shows my page status as text when it is linked", async () => {
    mockApi(
      student({
        page_linked: true,
        page: { linked: true, yearbook_id: BOOK, profile_complete: true, approved_notes: 12 },
      }),
    );
    renderApp(`/classes/${CID}`);
    expect(await screen.findByText("Profile complete")).toBeInTheDocument();
    expect(screen.getByText("Approved notes: 12")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open my page" })).toHaveAttribute(
      "href",
      `/yearbooks/${BOOK}`,
    );
    expect(screen.queryByRole("button", { name: "Create my page" })).not.toBeInTheDocument();
  });

  it("says the same for a missing class and a class I am not in", async () => {
    mockApi({ "GET /user/get-me": me, [`GET /class/get?id=${CID}`]: err(404, "not_found") });
    renderApp(`/classes/${CID}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("This class does not exist.");
    expect(screen.getByRole("link", { name: "Back to my classes" })).toBeInTheDocument();
  });

  it("leaves after a confirmation, and the last-admin rule has a clear message", async () => {
    let n = 0;
    const calls = mockApi({
      ...student({ my_role: "admin" }),
      [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
      [`GET /class-member/get-list?class_id=${CID}`]: list([]),
      "POST /class-member/leave": () => (n++ === 0 ? err(403, "last_admin") : ok()),
      "GET /class/get-list": list([]),
    });
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Leave class" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Leave class" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("at least one admin");
    await userEvent.click(within(dialog).getByRole("button", { name: "Leave class" }));
    await waitFor(() => expect(where()).toBe("/classes"));
    expect(calls.filter((c) => c.path === "/class-member/leave")).toHaveLength(2);
  });
});

describe("class screen, admin", () => {
  const admin = (extra: Record<string, object | (() => Response)> = {}) => ({
    "GET /user/get-me": me,
    [`GET /class/get?id=${CID}`]: ok(cls({ my_role: "admin" })),
    [`GET /class-member/get-list?class_id=${CID}`]: list([
      member(),
      member({
        id: "01J9Z3K6V8Q4M7N2P5R8T0MMM2",
        name: "Hà Lê",
        role: "admin",
        page_linked: false,
      }),
    ]),
    [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
    ...extra,
  });

  it("shows the roster with roles and page status as text", async () => {
    mockApi(admin());
    renderApp(`/classes/${CID}`);
    const roster = (await screen.findByRole("heading", { name: "Members" })).closest("section")!;
    const minh = (await within(roster).findByText("Minh Trần")).closest("li")!;
    expect(minh).toHaveTextContent("Role: Student");
    expect(minh).toHaveTextContent("Profile not complete yet");
    expect(minh).toHaveTextContent("Approved notes: 3");
    const ha = within(roster).getByText("Hà Lê").closest("li")!;
    expect(ha).toHaveTextContent("Role: Admin");
    expect(ha).toHaveTextContent("No page chosen yet");
    expect(roster.textContent).not.toMatch(/@/); // no email in the roster
  });

  it("promotes a student and shows a clear message for the last admin", async () => {
    let fail = false;
    const calls = mockApi(
      admin({
        "POST /class-member/update-role": () => (fail ? err(403, "last_admin") : ok()),
      }),
    );
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Make Minh Trần an admin" }));
    await waitFor(() =>
      expect(calls.find((c) => c.path === "/class-member/update-role")?.body).toEqual({
        class_id: CID,
        member_id: "01J9Z3K6V8Q4M7N2P5R8T0MMM1",
        role: "admin",
      }),
    );
    fail = true;
    await userEvent.click(screen.getByRole("button", { name: "Make Hà Lê a student" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at least one admin");
  });

  it("removes a member only after a confirmation", async () => {
    const calls = mockApi(admin({ "POST /class-member/remove": ok() }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Remove Minh Trần" }));
    expect(calls.some((c) => c.path === "/class-member/remove")).toBe(false);
    const dialog = screen.getByRole("dialog", { name: "Remove Minh Trần from the class?" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(calls.find((c) => c.path === "/class-member/remove")?.body).toEqual({
      class_id: CID,
      member_id: "01J9Z3K6V8Q4M7N2P5R8T0MMM1",
    });
  });

  it("creates an invite, shows the link once, and it is gone after a reload", async () => {
    const calls = mockApi(
      admin({
        "POST /class-invite/create": ok({ ...invite(), token: TOKEN, link: LINK }),
      }),
    );
    const log = vi.spyOn(console, "log");
    const warn = vi.spyOn(console, "warn");
    const error = vi.spyOn(console, "error");
    const first = renderApp(`/classes/${CID}`);
    await userEvent.selectOptions(await screen.findByLabelText("Link works for"), "30");
    await userEvent.click(screen.getByRole("button", { name: "Create invite link" }));
    expect(await screen.findByLabelText("Invite link")).toHaveValue(LINK);
    expect(screen.getByText(/cannot be shown again/)).toBeInTheDocument();
    const body = calls.find((c) => c.path === "/class-invite/create")?.body as {
      class_id: string;
      expires_at: number;
    };
    expect(body.class_id).toBe(CID);
    const days = (body.expires_at - Date.now()) / 86_400_000;
    expect(days).toBeGreaterThan(29.9);
    expect(days).toBeLessThanOrEqual(30); // the server refuses more than 30 days
    // the token is nowhere but the field: not in storage, not in the URL, not in the console
    const stored = JSON.stringify([{ ...localStorage }, { ...sessionStorage }]);
    expect(stored).not.toContain(TOKEN);
    expect(window.location.href).not.toContain(TOKEN);
    for (const spy of [log, warn, error])
      expect(JSON.stringify(spy.mock.calls)).not.toContain(TOKEN);
    expect(calls.every((c) => !c.path.includes(TOKEN))).toBe(true);
    // the list that follows has no token field
    first.unmount();
    renderApp(`/classes/${CID}`);
    await screen.findByRole("heading", { name: "Invite links" });
    expect(screen.queryByLabelText("Invite link")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain(TOKEN);
  });

  it("copies the link and can hide it", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    mockApi(admin({ "POST /class-invite/create": ok({ ...invite(), token: TOKEN, link: LINK }) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Create invite link" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy link" }));
    expect(writeText).toHaveBeenCalledWith(LINK);
    expect(await screen.findByText("Link copied.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Done, hide the link" }));
    expect(screen.queryByLabelText("Invite link")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain(TOKEN);
  });

  it("says so when copying is not possible", async () => {
    vi.stubGlobal("navigator", {
      ...navigator,
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    mockApi(admin({ "POST /class-invite/create": ok({ ...invite(), token: TOKEN, link: LINK }) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Create invite link" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy link" }));
    expect(await screen.findByText(/Could not copy/)).toBeInTheDocument();
  });

  it("explains the cap of 5 active links", async () => {
    mockApi(admin({ "POST /class-invite/create": err(409, "limit_reached") }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Create invite link" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("at most 5 active invite links");
  });

  it("lists invites with a text status and revokes an active one after a confirmation", async () => {
    const calls = mockApi(
      admin({
        [`GET /class-invite/get-list?class_id=${CID}`]: list([
          invite(),
          invite({ id: "01J9Z3K6V8Q4M7N2P5R8T0III2", active: false, revoked_at: 1760500000000 }),
          invite({ id: "01J9Z3K6V8Q4M7N2P5R8T0III3", active: false }),
        ]),
        "POST /class-invite/revoke": ok(),
      }),
    );
    renderApp(`/classes/${CID}`);
    const section = (await screen.findByRole("heading", { name: "Invite links" })).closest(
      "section",
    )!;
    expect(await within(section).findByText("Active")).toBeInTheDocument();
    expect(within(section).getByText("Revoked")).toBeInTheDocument();
    expect(within(section).getByText("Expired")).toBeInTheDocument();
    expect(within(section).getAllByRole("button", { name: "Revoke" })).toHaveLength(1);
    await userEvent.click(within(section).getByRole("button", { name: "Revoke" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Revoke" }),
    );
    await waitFor(() =>
      expect(calls.find((c) => c.path === "/class-invite/revoke")?.body).toEqual({
        class_id: CID,
        id: "01J9Z3K6V8Q4M7N2P5R8T0III1",
      }),
    );
  });

  it("works in Vietnamese", async () => {
    await i18n.changeLanguage("vi");
    mockApi(admin());
    renderApp(`/classes/${CID}`);
    expect(await screen.findByRole("heading", { name: "Thành viên" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Đường dẫn mời" })).toBeInTheDocument();
    expect(await screen.findByText("Vai trò: Học sinh", { exact: false })).toBeInTheDocument();
  });
});
