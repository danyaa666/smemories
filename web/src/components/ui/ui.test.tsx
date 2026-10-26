import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Badge } from "./Badge";
import { Banner } from "./Banner";
import { Button, buttonClass } from "./Button";
import { Dialog } from "./Dialog";
import { EmptyState } from "./EmptyState";
import { PhotoTile } from "./PhotoTile";
import { Skeleton } from "./Skeleton";
import { TextArea, TextField } from "./TextField";
import { Toast } from "./Toast";

describe("Button", () => {
  it("is a button of type button by default and fires on click and Enter", async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Save</Button>);
    const b = screen.getByRole("button", { name: "Save" });
    expect(b).toHaveAttribute("type", "button");
    await userEvent.click(b);
    b.focus();
    await userEvent.keyboard("{Enter}");
    expect(onClick).toHaveBeenCalledTimes(2);
  });

  it.each(["primary", "secondary", "ghost", "danger"] as const)("has the %s variant class", (v) => {
    render(<Button variant={v}>x</Button>);
    expect(screen.getByRole("button")).toHaveClass("btn", `btn-${v}`);
    expect(buttonClass(v)).toBe(`btn btn-${v}`);
  });

  it("is disabled and aria-busy while busy, and does not fire", async () => {
    const onClick = vi.fn();
    render(
      <Button busy onClick={onClick}>
        Save
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Save" });
    expect(b).toBeDisabled();
    expect(b).toHaveAttribute("aria-busy", "true");
    await userEvent.click(b);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("can be disabled without being busy", () => {
    render(<Button disabled>Save</Button>);
    expect(screen.getByRole("button")).not.toHaveAttribute("aria-busy");
    expect(screen.getByRole("button")).toBeDisabled();
  });
});

describe("TextField and TextArea", () => {
  it("labels the input and links help and error with aria-describedby", () => {
    render(
      <TextField
        label="Email"
        help="We never share it"
        error="Enter a valid email"
        value=""
        readOnly
      />,
    );
    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("We never share it Enter a valid email");
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a valid email");
  });

  it("has no alert and no aria-invalid without an error", () => {
    render(<TextField label="Email" value="" readOnly />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Email")).not.toHaveAttribute("aria-invalid");
    expect(screen.getByLabelText("Email")).not.toHaveAttribute("aria-describedby");
  });

  it("counts characters against maxLength and tells the reader", async () => {
    function Wrapper() {
      const [v, setV] = useState("");
      return (
        <TextArea
          label="Quote"
          maxLength={10}
          counter
          value={v}
          onChange={(e) => setV(e.target.value)}
        />
      );
    }
    render(<Wrapper />);
    expect(screen.getByText("0/10")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Quote"), "hello");
    expect(screen.getByText("5/10")).toBeInTheDocument();
    expect(screen.getByLabelText("Quote")).toHaveAccessibleDescription("5/10");
  });

  it("counts runes like the server: 10 emoji are 10, not 20", () => {
    render(<TextArea label="Quote" maxLength={4000} counter value={"😀".repeat(10)} readOnly />);
    expect(screen.getByText("10/4000")).toBeInTheDocument();
  });

  it("does not show a counter without maxLength", () => {
    render(<TextField label="Name" counter value="abc" readOnly />);
    expect(screen.queryByText(/\//)).not.toBeInTheDocument();
  });
});

describe("Badge", () => {
  it.each(["pending", "approved", "hidden"] as const)("shows its text with the %s style", (v) => {
    render(<Badge variant={v}>{v} label</Badge>);
    expect(screen.getByText(`${v} label`)).toHaveClass("badge", `badge-${v}`);
  });
});

describe("Toast", () => {
  it("is a status and its action is reachable and works with the keyboard", async () => {
    const onAction = vi.fn();
    render(<Toast message="Note hidden" actionLabel="Undo" onAction={onAction} />);
    expect(screen.getByRole("status")).toHaveTextContent("Note hidden");
    await userEvent.tab();
    expect(screen.getByRole("button", { name: "Undo" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(onAction).toHaveBeenCalledOnce();
  });

  it("has no button without an action", () => {
    render(<Toast message="Saved" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("Dialog", () => {
  function Harness({ onConfirm = vi.fn() }: { onConfirm?: () => void }) {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>Delete book</button>
        {open && (
          <Dialog
            title="Delete this book?"
            confirmLabel="Delete"
            cancelLabel="Keep it"
            danger
            onConfirm={onConfirm}
            onCancel={() => setOpen(false)}
          >
            This cannot be undone.
          </Dialog>
        )}
      </>
    );
  }

  it("is a named modal dialog and starts on the safe choice", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Delete book" }));
    const d = screen.getByRole("dialog", { name: "Delete this book?" });
    expect(d).toHaveAttribute("aria-modal", "true");
    expect(screen.getByRole("button", { name: "Keep it" })).toHaveFocus();
  });

  it("keeps Tab and Shift+Tab inside", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Delete book" }));
    await userEvent.tab();
    expect(screen.getByRole("button", { name: "Delete" })).toHaveFocus();
    await userEvent.tab();
    expect(screen.getByRole("button", { name: "Keep it" })).toHaveFocus();
    await userEvent.tab({ shift: true });
    expect(screen.getByRole("button", { name: "Delete" })).toHaveFocus();
  });

  it("closes on Escape and returns focus to the opener", async () => {
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Delete book" });
    await userEvent.click(opener);
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  // The export dialogs close on confirm while the opener is disabled for the running job (T-115).
  function BusyHarness({ finish }: { finish: { current?: () => void } }) {
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    finish.current = () => setBusy(false);
    return (
      <>
        <button disabled={busy} onClick={() => setOpen(true)}>
          Export
        </button>
        <button>Elsewhere</button>
        {open && (
          <Dialog
            title="Export?"
            confirmLabel="Go"
            cancelLabel="No"
            onConfirm={() => {
              setBusy(true);
              setOpen(false);
            }}
            onCancel={() => setOpen(false)}
          />
        )}
      </>
    );
  }

  it("returns focus to the opener after confirm once the running job enables it again", async () => {
    const finish: { current?: () => void } = {};
    render(<BusyHarness finish={finish} />);
    const opener = screen.getByRole("button", { name: "Export" });
    await userEvent.click(opener);
    await userEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(opener).toBeDisabled();
    act(() => finish.current?.());
    await waitFor(() => expect(opener).toHaveFocus());
  });

  it("leaves focus alone when the user has moved on before the job ends", async () => {
    const finish: { current?: () => void } = {};
    render(<BusyHarness finish={finish} />);
    await userEvent.click(screen.getByRole("button", { name: "Export" }));
    await userEvent.click(screen.getByRole("button", { name: "Go" }));
    await userEvent.click(screen.getByRole("button", { name: "Elsewhere" }));
    act(() => finish.current?.());
    await Promise.resolve();
    expect(screen.getByRole("button", { name: "Elsewhere" })).toHaveFocus();
  });

  it("leaves no timer, observer or focusin listener behind after the 30 s cap when the opener never re-enables (QA T-115)", async () => {
    const removed = vi.spyOn(document, "removeEventListener");
    const added = vi.spyOn(document, "addEventListener");
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const finish: { current?: () => void } = {};
      render(<BusyHarness finish={finish} />);
      await userEvent.click(screen.getByRole("button", { name: "Export" }));
      await userEvent.click(screen.getByRole("button", { name: "Go" }));
      expect(vi.getTimerCount()).toBe(1); // the give-up timer
      expect(added.mock.calls.filter(([t]) => t === "focusin")).toHaveLength(1);
      act(() => void vi.advanceTimersByTime(30_001));
      expect(vi.getTimerCount()).toBe(0);
      expect(removed.mock.calls.filter(([t]) => t === "focusin")).toHaveLength(1);
      act(() => finish.current?.()); // enabling it after the cap must not steal focus
      await Promise.resolve();
      expect(screen.getByRole("button", { name: "Export" })).not.toHaveFocus();
    } finally {
      vi.useRealTimers();
      removed.mockRestore();
      added.mockRestore();
    }
  });

  it("returns focus to the opener when a click outside the box closes the dialog", async () => {
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Delete book" });
    await userEvent.click(opener);
    await userEvent.click(screen.getByRole("dialog").parentElement!);
    expect(opener).toHaveFocus();
  });

  it("confirms only on the confirm button", async () => {
    const onConfirm = vi.fn();
    render(<Harness onConfirm={onConfirm} />);
    await userEvent.click(screen.getByRole("button", { name: "Delete book" }));
    expect(onConfirm).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("cancels on a click outside the box but not inside it", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Delete book" }));
    await userEvent.click(screen.getByText("This cannot be undone."));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("dialog").parentElement!);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

describe("Banner", () => {
  it("announces an error at once", () => {
    render(<Banner variant="error">Could not save</Banner>);
    expect(screen.getByRole("alert")).toHaveTextContent("Could not save");
  });

  it("announces offline politely and can carry an action", () => {
    render(
      <Banner variant="offline" action={<Button>Retry</Button>}>
        You are offline
      </Banner>,
    );
    expect(screen.getByRole("status")).toHaveTextContent("You are offline");
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });
});

describe("Skeleton", () => {
  it("is hidden from assistive technology and sized by its props", () => {
    const { container } = render(<Skeleton width={120} height="2rem" />);
    const el = container.firstElementChild as HTMLElement;
    expect(el).toHaveAttribute("aria-hidden", "true");
    expect(el.style.width).toBe("120px");
    expect(el.style.height).toBe("2rem");
  });
});

describe("EmptyState", () => {
  it("shows a heading, text and an action", () => {
    render(
      <EmptyState title="No notes yet" action={<Button>Share link</Button>}>
        Share your link.
      </EmptyState>,
    );
    expect(screen.getByRole("heading", { name: "No notes yet" })).toBeInTheDocument();
    expect(screen.getByText("Share your link.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Share link" })).toBeInTheDocument();
  });
});

describe("PhotoTile", () => {
  it("shows the photo with its alt text and tape only as decoration", () => {
    const { container } = render(<PhotoTile src="/p.jpg" alt="Photo 1" tape />);
    expect(screen.getByRole("img", { name: "Photo 1" })).toHaveAttribute("src", "/p.jpg");
    expect(container.querySelector(".tape")).toHaveAttribute("aria-hidden", "true");
  });

  it("has a remove button named by the caller, only when asked", async () => {
    const onRemove = vi.fn();
    const { rerender } = render(<PhotoTile src="/p.jpg" alt="Photo 1" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    rerender(
      <PhotoTile src="/p.jpg" alt="Photo 1" onRemove={onRemove} removeLabel="Remove photo 1" />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Remove photo 1" }));
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("renders children inside the frame and disables remove", () => {
    render(
      <PhotoTile src="/p.jpg" alt="x" onRemove={() => {}} removeLabel="Remove" disabled>
        <button>Cover</button>
      </PhotoTile>,
    );
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cover" })).toBeInTheDocument();
  });
});
