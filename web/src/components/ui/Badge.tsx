import type { ReactNode } from "react";

export type BadgeVariant = "pending" | "approved" | "hidden" | "error";

/** A status pill. The text must say the status: colour alone is not enough. */
export function Badge({ variant, children }: { variant: BadgeVariant; children: ReactNode }) {
  return <span className={`badge badge-${variant}`}>{children}</span>;
}
