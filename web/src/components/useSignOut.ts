import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { logout } from "../api/auth";
import { ME_KEY } from "../auth/useMe";

/** Sign out: nothing cached for the old user survives, then the login page. */
export function useSignOut() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: logout,
    onSuccess: () => {
      qc.clear();
      qc.setQueryData(ME_KEY, null);
      void navigate("/login");
    },
  });
}
