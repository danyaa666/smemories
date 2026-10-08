import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";
import i18n from "./i18n";

function renderAt(path: string) {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

// Everything except the session probe gets `res`; the probe says "signed out".
const respond = (res: () => Promise<Response>) =>
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      url === "/api/v1/me"
        ? Response.json(
            { error: { code: "unauthenticated", message: "", request_id: "" } },
            { status: 401 },
          )
        : res(),
    ),
  );

describe("App", () => {
  it("shows header and main landmarks, the app name and the tagline", async () => {
    respond(async () => Response.json({ status: "ok" }));
    renderAt("/");
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "SMemories" })).toBeInTheDocument();
    expect(screen.getByText("Design, collect and print your own yearbook.")).toBeInTheDocument();
    expect(await screen.findByText("API: ok")).toBeInTheDocument();
  });

  it("shows 'API: unreachable' on a network error", async () => {
    respond(async () => {
      throw new TypeError("Failed to fetch");
    });
    renderAt("/");
    expect(await screen.findByText("API: unreachable")).toBeInTheDocument();
  });

  it("shows 'API: unreachable' on a 500 error envelope", async () => {
    respond(async () =>
      Response.json(
        { error: { code: "internal_error", message: "boom", request_id: "r1" } },
        { status: 500 },
      ),
    );
    renderAt("/");
    expect(await screen.findByText("API: unreachable")).toBeInTheDocument();
  });

  it("translates the badge and tagline to Vietnamese", async () => {
    window.localStorage.setItem("smemories.lang", "vi");
    await i18n.changeLanguage("vi");
    respond(async () => Response.json({ status: "ok" }));
    renderAt("/");
    expect(await screen.findByText("API: hoạt động")).toBeInTheDocument();
    expect(screen.getByText("Thiết kế, thu thập và in kỷ yếu của riêng bạn.")).toBeInTheDocument();
  });

  it("renders a translated not-found page for an unknown route", () => {
    renderAt("/nope");
    expect(screen.getByRole("heading", { name: "Page not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the home page" })).toHaveAttribute(
      "href",
      "/",
    );
  });
});
