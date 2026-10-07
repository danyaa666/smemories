import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { LanguageSwitcher } from "./LanguageSwitcher";

describe("LanguageSwitcher", () => {
  it("is a labelled group with the active language pressed", () => {
    render(<LanguageSwitcher />);
    expect(screen.getByRole("group", { name: "Language" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "EN" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "VI" })).toHaveAttribute("aria-pressed", "false");
  });

  it("switches on click, relabels the group, persists and sets <html lang>", async () => {
    render(<LanguageSwitcher />);
    await userEvent.click(screen.getByRole("button", { name: "VI" }));
    expect(screen.getByRole("button", { name: "VI" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("group", { name: "Ngôn ngữ" })).toBeInTheDocument();
    expect(window.localStorage.getItem("smemories.lang")).toBe("vi");
    expect(document.documentElement.lang).toBe("vi");
  });

  it("works from the keyboard", async () => {
    render(<LanguageSwitcher />);
    await userEvent.tab();
    expect(screen.getByRole("button", { name: "EN" })).toHaveFocus();
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "VI" })).toHaveAttribute("aria-pressed", "true");
  });
});
