import type { ReactNode } from "react";

/** A full-width message: "offline" is polite (role="status"), "error" is announced at once (role="alert"). */
export function Banner({
  variant,
  children,
  action,
}: {
  variant: "offline" | "error";
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className={`banner banner-${variant}`} role={variant === "error" ? "alert" : "status"}>
      <span className="banner-text">{children}</span>
      {action}
    </div>
  );
}
