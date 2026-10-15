import type { ReactNode } from "react";

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <h2 className="empty-title">{title}</h2>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}
