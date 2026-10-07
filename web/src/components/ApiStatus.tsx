import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getHealth } from "../api/client";

export function ApiStatus() {
  const { t } = useTranslation();
  const { status } = useQuery({ queryKey: ["healthz"], queryFn: getHealth, retry: false });
  const state = status === "success" ? "ok" : status === "error" ? "unreachable" : "checking";
  return (
    <p role="status" className={`badge badge-${state}`}>
      {t(`api.${state}`)}
    </p>
  );
}
