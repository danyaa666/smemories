import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { getClass } from "../api/classes";
import { ApiError } from "../api/client";
import { errorText } from "../auth/errors";
import { Button } from "../components/ui/Button";
import { NotFound } from "../pages/NotFound";

/**
 * Admin-only screens (assemble, print): the class is read first, and a student or a stranger gets the same not-found screen as any
 * unknown address, without the screen's own requests (the class book) ever being made.
 */
export function AdminOnly({ children }: { children: (classId: string) => ReactNode }) {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  const q = useQuery({ queryKey: ["class", id], queryFn: () => getClass(id) });
  if (q.isPending) return <p role="status">{t("auth.loading")}</p>;
  if (q.isError) {
    if (q.error instanceof ApiError && (q.error.status === 404 || q.error.status === 400))
      return <NotFound />;
    return (
      <>
        <p role="alert" className="error">
          {errorText(t, q.error)}
        </p>
        <Button onClick={() => void q.refetch()}>{t("auth.retry")}</Button>
      </>
    );
  }
  if (q.data.my_role !== "admin") return <NotFound />;
  return <>{children(q.data.id)}</>;
}
