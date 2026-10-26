import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ClassMember, SchoolClass } from "../api/classes";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const CID = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
const B1 = "01J9Z3K6V8Q4M7N2P5R8T0BK01";
const B2 = "01J9Z3K6V8Q4M7N2P5R8T0BK02";
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
const status = (id: string | null) =>
  ok({ linked: id !== null, yearbook_id: id, profile_complete: false, approved_notes: 0 });
const book = (id: string, title: string) => ({
  id,
  title,
  school_name: "",
  class_name: "",
  graduation_year: null,
  motto: "",
  language: "en",
  page_size: "A5",
  template_id: null,
  cover_media_id: null,
  created_at: 1,
  updated_at: 1,
});
const list = (items: object[]) => ok({ items, next_id: "" });
const member = (id: string, name: string, role: ClassMember["role"]): ClassMember => ({
  id,
  name,
  role,
  joined_at: 1760000000000,
  page_linked: false,
  profile_complete: false,
  approved_notes: 0,
});
const CLASS = `GET /class/get?id=${CID}`;

describe("edit and delete a class", () => {
  const admin = (extra: Record<string, object | (() => Response)> = {}) => ({
    "GET /user/get-me": () => ok(user()),
    [CLASS]: ok(cls({ my_role: "admin" })),
    [`GET /class-member/get-list?class_id=${CID}`]: list([]),
    [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
    ...extra,
  });

  it("shows the form filled in, saves with class/update and confirms", async () => {
    const saved = cls({
      my_role: "admin",
      name: "Lớp 12A2",
      school_name: "",
      graduation_year: null,
    });
    const calls = mockApi(
      admin({ "POST /class/update": ok(saved), "GET /class/get-list": list([]) }),
    );
    renderApp(`/classes/${CID}`);
    const name = await screen.findByLabelText("Class name");
    expect(name).toHaveValue("Lớp 12A1");
    expect(screen.getByLabelText("School")).toHaveValue("THPT Chu Văn An");
    expect(screen.getByLabelText("Graduation year")).toHaveValue(2026);
    expect(screen.getByLabelText("Language of the class book")).toHaveValue("vi");
    await userEvent.clear(name);
    await userEvent.type(name, "Lớp 12A2");
    await userEvent.clear(screen.getByLabelText("School"));
    await userEvent.clear(screen.getByLabelText("Graduation year"));
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    expect(calls.find((c) => c.path === "/class/update")?.body).toEqual({
      id: CID,
      name: "Lớp 12A2",
      school_name: "",
      graduation_year: null,
      language: "vi",
    });
    expect(await screen.findByRole("heading", { name: "Lớp 12A2" })).toBeInTheDocument();
  });

  it("puts a server error on its field", async () => {
    mockApi(admin({ "POST /class/update": err(400, "invalid_name") }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    expect(await screen.findByLabelText("Class name")).toHaveAccessibleDescription(
      /class name of 1 to 120/,
    );
  });

  it("deletes only after a confirmation that names the class and what happens, then returns to the list", async () => {
    const calls = mockApi(admin({ "POST /class/delete": ok(), "GET /class/get-list": list([]) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Delete class" }));
    expect(calls.some((c) => c.path === "/class/delete")).toBe(false);
    const dialog = screen.getByRole("dialog", { name: "Delete Lớp 12A1?" });
    expect(dialog).toHaveTextContent("Nobody's yearbook is deleted");
    expect(dialog).toHaveTextContent("loses access");
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete class" }));
    await waitFor(() => expect(where()).toBe("/classes"));
    expect(calls.find((c) => c.path === "/class/delete")?.body).toEqual({ id: CID });
  });

  it("keeps the dialog open and says why when the delete fails; cancel changes nothing", async () => {
    const calls = mockApi(admin({ "POST /class/delete": err(403, "forbidden") }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Delete class" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete class" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Only a class admin");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(where()).toBe(`/classes/${CID}`);
    expect(calls.filter((c) => c.path === "/class/delete")).toHaveLength(1);
  });

  it("is in Vietnamese too", async () => {
    const { default: i18n } = await import("../i18n");
    await i18n.changeLanguage("vi");
    mockApi(admin());
    renderApp(`/classes/${CID}`);
    expect(await screen.findByRole("button", { name: "Xóa lớp" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "In kỷ yếu lớp" })).toBeInTheDocument();
  });

  it("gives a student none of it: nothing rendered, nothing requested", async () => {
    const calls = mockApi({ "GET /user/get-me": () => ok(user()), [CLASS]: ok(cls()) });
    renderApp(`/classes/${CID}`);
    await screen.findByRole("heading", { name: "Lớp 12A1" });
    expect(screen.queryByLabelText("Class name")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Save changes|Delete class/ })).toBeNull();
    expect(screen.queryByRole("link", { name: "Print the class book" })).toBeNull();
    expect(calls.filter((c) => c.method === "POST")).toEqual([]);
  });
});

describe("print link", () => {
  it("points the admin to the book from the class and from the assemble screen", async () => {
    mockApi({
      "GET /user/get-me": () => ok(user()),
      [CLASS]: ok(cls({ my_role: "admin" })),
      [`GET /class-member/get-list?class_id=${CID}`]: list([]),
      [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
      [`GET /class-book/get?class_id=${CID}`]: ok({
        class: {
          id: CID,
          name: "Lớp 12A1",
          school_name: "",
          graduation_year: 2026,
          language: "vi",
        },
        front_pages: [],
        members: [],
        back_pages: [],
      }),
    });
    renderApp(`/classes/${CID}`);
    const link = await screen.findByRole("link", { name: "Print the class book" });
    expect(link).toHaveAttribute("href", `/classes/${CID}/book`);
    await userEvent.click(screen.getByRole("link", { name: "Assemble the class book" }));
    expect(await screen.findByRole("link", { name: "Print the class book" })).toHaveAttribute(
      "href",
      `/classes/${CID}/book`,
    );
    // the two plain links get a 44 px target through these classes (styles.css)
    expect(screen.getByRole("link", { name: "Back to the class" })).toHaveClass("back-link");
  });
});

describe("page picker", () => {
  const withBooks = (
    page: SchoolClass["page"],
    extra: Record<string, object | (() => Response)> = {},
  ) => ({
    "GET /user/get-me": () => ok(user()),
    [CLASS]: ok(cls(page ? { page_linked: true, page } : {})),
    "GET /yearbook/get-list": list([book(B1, "My first book"), book(B2, "Second book")]),
    ...extra,
  });
  const linkedTo = (id: string) => ({
    linked: true,
    yearbook_id: id,
    profile_complete: true,
    approved_notes: 2,
  });

  it("does not request the yearbook list until the picker is opened", async () => {
    const calls = mockApi(withBooks(undefined));
    renderApp(`/classes/${CID}`);
    await screen.findByRole("button", { name: "Choose an existing yearbook" });
    expect(calls.some((c) => c.path === "/yearbook/get-list")).toBe(false);
  });

  it("links an existing yearbook as my page", async () => {
    const calls = mockApi(withBooks(undefined, { "POST /class-member/set-book": status(B2) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(
      await screen.findByRole("button", { name: "Choose an existing yearbook" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Use Second book as my page" }),
    );
    await waitFor(() =>
      expect(calls.find((c) => c.path === "/class-member/set-book")?.body).toEqual({
        class_id: CID,
        yearbook_id: B2,
      }),
    );
    await waitFor(() => expect(screen.queryByText("Choose one of your yearbooks")).toBeNull());
  });

  it("switches to another yearbook and marks the current one", async () => {
    const calls = mockApi(withBooks(linkedTo(B1), { "POST /class-member/set-book": status(B2) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Choose another yearbook" }));
    const current = (await screen.findByText("My first book")).closest("li")!;
    expect(current).toHaveTextContent("This is your page now.");
    expect(within(current).queryByRole("button")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Use Second book as my page" }));
    await waitFor(() =>
      expect(calls.find((c) => c.path === "/class-member/set-book")?.body).toEqual({
        class_id: CID,
        yearbook_id: B2,
      }),
    );
  });

  it("shows why a yearbook linked elsewhere is unavailable, and keeps the other choice", async () => {
    mockApi(withBooks(undefined, { "POST /class-member/set-book": err(409, "conflict") }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(
      await screen.findByRole("button", { name: "Choose an existing yearbook" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Use My first book as my page" }),
    );
    const row = (await screen.findByText("My first book")).closest("li")!;
    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "already the page of another class",
    );
    expect(within(row).queryByRole("button")).toBeNull();
    expect(screen.getByRole("button", { name: "Use Second book as my page" })).toBeEnabled();
  });

  it("unlinks with null", async () => {
    const calls = mockApi(withBooks(linkedTo(B1), { "POST /class-member/set-book": status(null) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Unlink my page" }));
    await waitFor(() =>
      expect(calls.find((c) => c.path === "/class-member/set-book")?.body).toEqual({
        class_id: CID,
        yearbook_id: null,
      }),
    );
  });

  it("says so when there is no yearbook to pick", async () => {
    mockApi(withBooks(undefined, { "GET /yearbook/get-list": list([]) }));
    renderApp(`/classes/${CID}`);
    await userEvent.click(
      await screen.findByRole("button", { name: "Choose an existing yearbook" }),
    );
    expect(await screen.findByText("You have no yearbooks yet.")).toBeInTheDocument();
  });
});

describe("role-change errors", () => {
  it("show next to the member's row, not above the list", async () => {
    mockApi({
      "GET /user/get-me": () => ok(user()),
      [CLASS]: ok(cls({ my_role: "admin" })),
      [`GET /class-invite/get-list?class_id=${CID}`]: list([]),
      [`GET /class-member/get-list?class_id=${CID}`]: list([
        member("01J9Z3K6V8Q4M7N2P5R8T0MMM1", "Minh Trần", "student"),
        member("01J9Z3K6V8Q4M7N2P5R8T0MMM2", "Hà Lê", "admin"),
      ]),
      "POST /class-member/update-role": err(403, "last_admin"),
    });
    renderApp(`/classes/${CID}`);
    await userEvent.click(await screen.findByRole("button", { name: "Make Hà Lê a student" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("at least one admin");
    expect(alert.closest("li")).toHaveTextContent("Hà Lê");
    expect(within(screen.getByText("Minh Trần").closest("li")!).queryByRole("alert")).toBeNull();
  });
});
