import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Link, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import "../i18n";
import { MAIN_ID, RouteFocus, SkipLink } from "./RouteA11y";

function Shell() {
  return (
    <MemoryRouter initialEntries={["/a"]}>
      <SkipLink />
      <RouteFocus />
      <Link to="/b">go b</Link>
      <Link to="/late">go late</Link>
      <main id={MAIN_ID} tabIndex={-1}>
        <Routes>
          <Route path="/a" element={<h1>Page A</h1>} />
          <Route path="/b" element={<h1>Page B</h1>} />
          <Route path="/late" element={<Late />} />
        </Routes>
      </main>
    </MemoryRouter>
  );
}

function Late() {
  // The heading appears after the route renders, like a page that waits for its data.
  return (
    <button
      onClick={(e) =>
        e.currentTarget.replaceWith(
          Object.assign(document.createElement("h1"), { textContent: "Late page" }),
        )
      }
    >
      show
    </button>
  );
}

describe("skip link and route focus", () => {
  it("skip link is the first Tab stop and moves focus to <main>", async () => {
    const u = userEvent.setup();
    render(<Shell />);
    await u.tab();
    const skip = screen.getByRole("link", { name: "Skip to content" });
    expect(skip).toHaveFocus();
    await u.keyboard("{Enter}");
    expect(screen.getByRole("main")).toHaveFocus();
  });

  it("does not move focus on the first load", () => {
    render(<Shell />);
    expect(document.body).toHaveFocus();
  });

  it("focuses the new <h1> after a navigation, and not for the same path", async () => {
    const u = userEvent.setup();
    render(<Shell />);
    await u.click(screen.getByRole("link", { name: "go b" }));
    const h1 = screen.getByRole("heading", { level: 1, name: "Page B" });
    expect(h1).toHaveFocus();
    expect(h1).toHaveAttribute("tabindex", "-1");
  });

  it("waits for an <h1> that renders later", async () => {
    const u = userEvent.setup();
    render(<Shell />);
    await u.click(screen.getByRole("link", { name: "go late" }));
    expect(document.querySelector("h1")).toBeNull();
    await u.click(screen.getByRole("button", { name: "show" }));
    await act(async () => {});
    expect(screen.getByRole("heading", { name: "Late page" })).toHaveFocus();
  });
});
