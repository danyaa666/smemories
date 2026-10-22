import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "../queryClient";
import i18n from "../i18n";
import { err, mockApi, renderApp, user } from "../test/api";
import { YearbookLayout, type NavItem } from "./YearbookLayout";

const ID = "01J9Z3K6V8Q4M7N2P5R8T0W1XZ";
const book = {
  id: ID,
  title: "Class 12A1",
  school_name: "Le Hong Phong",
  class_name: "12A1",
  graduation_year: 2026,
  motto: "",
  language: "en",
  page_size: "A5",
  template_id: null,
  cover_media_id: null,
  profile: {
    id: "01J9Z3K6V8Q4M7N2P5R8T0BBBB",
    is_owner: true,
    full_name: "",
    nickname: "",
    birthday: null,
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
  created_at: 1760000000000,
  updated_at: 1760086400000,
};

/** matchMedia that answers the (min-width: 960px) query for a window of this width. */
function viewport(width: number) {
  vi.stubGlobal("matchMedia", (q: string) => ({
    matches: width >= Number(/min-width:\s*(\d+)px/.exec(q)?.[1]),
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

afterEach(() => vi.unstubAllGlobals());

describe("YearbookLayout", () => {
  it("shows the sidebar, not the tab bar, at 1280 px", async () => {
    viewport(1280);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user() }),
      "GET /yearbook/get-list": Response.json({ status: "OK", data: { items: [], next_id: "" } }),
    });
    renderApp("/yearbooks");
    const nav = await screen.findByRole("navigation", { name: "Yearbook" });
    expect(within(nav).getByRole("link", { name: "My yearbooks" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("complementary")).toBeInTheDocument();
    expect(screen.queryByRole("banner")).not.toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
    // brand, chip, sign out and language switcher live in the sidebar
    expect(
      within(screen.getByRole("complementary")).getByRole("button", { name: "Sign out" }),
    ).toBeInTheDocument();
    expect(
      within(screen.getByRole("complementary")).getByRole("group", { name: "Language" }),
    ).toBeInTheDocument();
  });

  it("shows the tab bar, not the sidebar, at 375 px", async () => {
    viewport(375);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user() }),
      "GET /yearbook/get-list": Response.json({ status: "OK", data: { items: [], next_id: "" } }),
    });
    renderApp("/yearbooks");
    const nav = await screen.findByRole("navigation", { name: "Yearbook" });
    expect(within(nav).getByRole("link", { name: "Books" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
    expect(screen.getByRole("banner")).toBeInTheDocument();
  });

  it("on a book, shows the book card and the profile item (sidebar) or tab", async () => {
    viewport(1280);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user() }),
      [`GET /yearbook/get?id=${ID}`]: Response.json({ status: "OK", data: book }),
      [`GET /media/get-list?yearbook_id=${ID}`]: Response.json({
        status: "OK",
        data: { items: [], next_id: "" },
      }),
    });
    renderApp(`/yearbooks/${ID}`);
    // wait for the loaded page first (cheap heading query), then the shell card is already there
    await screen.findByRole("heading", { level: 1, name: "Class 12A1" });
    const card = screen.getByRole("link", { name: /Class 12A1.*Le Hong Phong/ });
    expect(card).toHaveAttribute("href", `/yearbooks/${ID}`);
    const nav = screen.getByRole("navigation", { name: "Yearbook" });
    expect(within(nav).getByRole("link", { name: "My profile page" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("hides items whose route does not exist: none of the later ones is listed", async () => {
    viewport(1280);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user() }),
      "GET /yearbook/get-list": Response.json({ status: "OK", data: { items: [], next_id: "" } }),
    });
    renderApp("/yearbooks");
    const nav = await screen.findByRole("navigation", { name: "Yearbook" });
    for (const name of ["Photos", "Note links", "Notes from friends", "Preview and print"]) {
      expect(within(nav).queryByRole("link", { name })).not.toBeInTheDocument();
    }
    // and "My profile page" needs an open book
    expect(within(nav).queryByRole("link", { name: "My profile page" })).not.toBeInTheDocument();
  });

  it("shows the username in the user chip", async () => {
    viewport(1280);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user({ username: "lan.nguyen" }) }),
      "GET /yearbook/get-list": Response.json({ status: "OK", data: { items: [], next_id: "" } }),
    });
    renderApp("/yearbooks");
    expect(await screen.findByRole("link", { name: "lan.nguyen" })).toBeInTheDocument();
  });

  it("takes extra items and shows a pending count badge", async () => {
    viewport(1280);
    const items: NavItem[] = [
      {
        key: "notes",
        to: () => "/yearbooks",
        label: "nav.yearbooks",
        tab: "nav.tabBooks",
        badge: 3,
      },
    ];
    mockApi({ "GET /user/get-me": Response.json({ status: "OK", data: user() }) });
    render(
      <QueryClientProvider client={makeQueryClient()}>
        <MemoryRouter initialEntries={["/yearbooks"]}>
          <Routes>
            <Route element={<YearbookLayout items={items} />}>
              <Route path="/yearbooks" element={<p>list</p>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("link", { name: "My yearbooks 3" })).toBeInTheDocument();
  });
});

describe("YearbookLayout: a missing or malformed book id", () => {
  // 404: no such book; 400 ERROR_PARAM: the id is not canonical (lower-cased, hand-typed), since T-100
  const answers = [
    [404, "not_found"],
    [400, "invalid_body"],
  ] as const;
  const screens = ["", "/notes", "/note-links", "/book"];

  it.each(answers.flatMap(([status, code]) => screens.map((s) => [status, code, s] as const)))(
    "%i %s on /yearbooks/:id%s shows 'book not found' and mounts no screen",
    async (status, code, screen_) => {
      viewport(1280);
      const calls = mockApi({
        "GET /user/get-me": Response.json({ status: "OK", data: user() }),
        [`GET /yearbook/get?id=${ID}`]: err(status, code),
      });
      renderApp(`/yearbooks/${ID}${screen_}`);
      expect(await screen.findByRole("alert")).toHaveTextContent("This yearbook does not exist.");
      expect(screen.getByRole("link", { name: "Back to my yearbooks" })).toHaveAttribute(
        "href",
        "/yearbooks",
      );
      expect(screen.queryByText("Something went wrong. Please try again.")).not.toBeInTheDocument();
      expect(calls.map((c) => c.path)).not.toContain("/note-collection/get-list");
      expect(calls.map((c) => c.path)).not.toContain("/book/get-data");
    },
  );

  it("says it in Vietnamese too", async () => {
    viewport(1280);
    await i18n.changeLanguage("vi");
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user({ locale: "vi" }) }),
      [`GET /yearbook/get?id=${ID}`]: err(400, "invalid_body"),
    });
    renderApp(`/yearbooks/${ID}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("Kỷ yếu này không tồn tại.");
  });

  it("keeps the generic error for a server failure", async () => {
    viewport(1280);
    mockApi({
      "GET /user/get-me": Response.json({ status: "OK", data: user() }),
      [`GET /yearbook/get?id=${ID}`]: err(500, "internal_error"),
    });
    renderApp(`/yearbooks/${ID}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("Something went wrong");
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});
