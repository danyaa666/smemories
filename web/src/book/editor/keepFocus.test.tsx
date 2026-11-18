import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { useKeepFocus } from "./keepFocus";

// T-182 (F-01): a button that disables itself while the keyboard is on it hands the focus to its neighbour instead of dropping it.
afterEach(cleanup);

function Bar() {
  useKeepFocus();
  const [at, setAt] = useState(1);
  return (
    <div role="toolbar" aria-label="bar">
      <button type="button" disabled={at === 0} onClick={() => setAt(0)}>
        Undo
      </button>
      <button type="button" disabled={at !== 0}>
        Redo
      </button>
      <button type="button" disabled>
        Off
      </button>
      <button type="button">Plain</button>
    </div>
  );
}

describe("useKeepFocus", () => {
  it("moves the focus to the nearest enabled button when the focused one becomes disabled", async () => {
    render(<Bar />);
    screen.getByRole("button", { name: "Undo" }).focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toHaveFocus();
  });

  it("does not move the focus when the button stays enabled, or after a pointer press elsewhere", async () => {
    render(<Bar />);
    screen.getByRole("button", { name: "Plain" }).focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Plain" })).toHaveFocus();
  });
});
