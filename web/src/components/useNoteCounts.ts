import { useQuery } from "@tanstack/react-query";
import { listNotes } from "../api/notes";

export const noteCountsKey = (yearbookId: string | undefined) =>
  ["note-counts", yearbookId] as const;

/** Notes of the whole book per status (the inbox tabs and the sidebar badge); the inbox keeps it up to date. */
export const useNoteCounts = (yearbookId: string | undefined) =>
  useQuery({
    queryKey: noteCountsKey(yearbookId),
    queryFn: async () => (await listNotes(yearbookId ?? "", undefined, undefined, 1)).counts,
    enabled: yearbookId !== undefined,
    staleTime: 30_000,
    retry: false, // a badge is not worth a retry; 4xx (unverified email) is a normal answer
  });
