import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { getHealth } from "../api/client";
import { Badge } from "./ui/Badge";

const BADGE = { checking: "pending", ok: "approved", unreachable: "error" } as const;

export function ApiStatus() {
  const { t } = useTranslation();
  const { status } = useQuery({ queryKey: ["healthz"], queryFn: getHealth, retry: false });
  const state = status === "success" ? "ok" : status === "error" ? "unreachable" : "checking";
  return (
    <p role="status">
      <Badge variant={BADGE[state]}>{t(`api.${state}`)}</Badge>
    </p>
  );
}
