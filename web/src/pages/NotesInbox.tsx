import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  BATCH_MAX,
  deleteNotes,
  listNotes,
  reorderNotes,
  retryOnce,
  updateNoteStatus,
  type NoteCounts,
  type NoteItem,
  type NotePage,
  type NoteStatus,
} from "../api/notes";
import { errorText } from "../auth/errors";
import { NoteCard, OrderRow, noteName, type DragState } from "../components/NoteCard";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";
import { EmptyState } from "../components/ui/EmptyState";
import { Skeleton } from "../components/ui/Skeleton";
import { Toast } from "../components/ui/Toast";
import { noteCountsKey, useNoteCounts } from "../components/useNoteCounts";
import { useMediaQuery } from "../components/useMediaQuery";
import { useOrderSave } from "../components/useOrderSave";

type Filter = "all" | NoteStatus;
const FILTERS: Filter[] = ["all", "pending", "approved", "hidden"];
/** The delete is sent when this runs out; Undo cancels it. Leaving the page drops it. */
export const UNDO_MS = 6000;
const SAVE_ORDER_MS = 700;
const TOAST_MS = 5000;

const arrayMove = <T,>(a: T[], from: number, to: number) => {
  const b = [...a];
  const [x] = b.splice(from, 1);
  b.splice(to, 0, x as T);
  return b;
};

function patchStatus(
  d: InfiniteData<NotePage>,
  filter: Filter,
  ids: string[],
  status: NoteStatus,
): InfiniteData<NotePage> {
  const set = new Set(ids);
  return {
    ...d,
    pages: d.pages.map((p) => ({
      ...p,
      // in a filtered tab a note that changes status leaves; the position is the server's to say
      items: p.items.flatMap((n) =>
        !set.has(n.id)
          ? [n]
          : filter !== "all" && filter !== status
            ? []
            : [{ ...n, status, position: null }],
      ),
    })),
  };
}

function shiftCounts(c: NoteCounts, from: NoteItem[], to: NoteStatus | null): NoteCounts {
  const out = { ...c };
  for (const n of from) {
    if (n.status === to) continue;
    out[n.status]--;
    if (to) out[to]++;
    else out.all--;
  }
  return out;
}

type ToastState = { message: string };

export function NotesInbox() {
  const { id = "" } = useParams();
  return <Inbox key={id} bookId={id} />;
}

function Inbox({ bookId }: { bookId: string }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const wide = useMediaQuery("(min-width: 960px)");
  const [filter, setFilter] = useState<Filter>("all");
  const [sel, setSel] = useState<string[]>([]);
  const [confirm, setConfirm] = useState<string[] | null>(null);
  const [ordering, setOrdering] = useState(false);
  const [drag, setDrag] = useState<DragState>(null);
  const [announce, setAnnounce] = useState("");
  const [toast, setToast] = useState<ToastState | null>(null);
  /** Notes the owner deleted whose Undo window is still open: gone from the screen, not yet from the server. */
  const [pending, setPending] = useState<NoteItem[]>([]);
  const toastTimer = useRef<number>(undefined);
  const delTimer = useRef<number>(undefined);
  const inflight = useRef(new Set<string>()); // ids whose batch-delete request is on its way

  const key = ["notes", bookId, filter] as const;
  const countsKey = noteCountsKey(bookId);
  const list = useInfiniteQuery({
    queryKey: key,
    queryFn: ({ pageParam }) => listNotes(bookId, filter === "all" ? undefined : filter, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_id || undefined,
    // a 4xx (not your book, unverified email) is the answer; retrying only delays it
    retry: retryOnce,
  });
  const gone = new Set(pending.map((n) => n.id));
  const items = (list.data?.pages.flatMap((p) => p.items) ?? []).filter((n) => !gone.has(n.id));
  const lastCounts = list.data?.pages.at(-1)?.counts;
  const countsQuery = useNoteCounts(bookId);
  const serverCounts = countsQuery.data ?? lastCounts;
  const counts = serverCounts && shiftCounts(serverCounts, pending, null);

  // the list is the freshest source of the counts: share it with the tabs, the header and the sidebar badge
  useEffect(() => {
    if (lastCounts) qc.setQueryData(countsKey, lastCounts);
  }, [lastCounts, qc, countsKey]);

  // reordering needs the whole approved set (at most 300), so that tab loads every page
  const { hasNextPage, isFetchingNextPage, isFetchNextPageError, fetchNextPage } = list;
  useEffect(() => {
    if (filter === "approved" && hasNextPage && !isFetchingNextPage && !isFetchNextPageError)
      void fetchNextPage();
  }, [filter, hasNextPage, isFetchingNextPage, isFetchNextPageError, fetchNextPage]);

  function showToast(next: ToastState, ms = TOAST_MS) {
    window.clearTimeout(toastTimer.current);
    setToast(next);
    toastTimer.current = window.setTimeout(() => setToast(null), ms);
  }

  // ── order ──
  const order = useOrderSave(async (ids) => {
    try {
      await reorderNotes(bookId, ids);
    } catch (e) {
      void qc.invalidateQueries({ queryKey: ["notes", bookId] });
      showToast({
        message:
          e instanceof ApiError && e.code === "order_stale" ? t("inbox.stale") : errorText(t, e),
      });
    }
  }, SAVE_ORDER_MS);

  // ── status ──
  const change = useMutation({
    mutationFn: async (v: { ids: string[]; status: NoteStatus }) => {
      await order.flush(); // the server must know the new order before the set of approved notes changes
      return updateNoteStatus(bookId, v.ids, v.status);
    },
    onMutate: async ({ ids, status }) => {
      await qc.cancelQueries({ queryKey: ["notes", bookId] });
      qc.removeQueries({ queryKey: ["notes", bookId], type: "inactive" }); // other tabs would show the old status
      const prevList = qc.getQueryData<InfiniteData<NotePage>>(key);
      const prevCounts = qc.getQueryData<NoteCounts>(countsKey);
      const moved = items.filter((n) => ids.includes(n.id));
      qc.setQueryData<InfiniteData<NotePage>>(key, (d) => d && patchStatus(d, filter, ids, status));
      qc.setQueryData<NoteCounts>(countsKey, (c) => c && shiftCounts(c, moved, status));
      setSel((s) => s.filter((x) => !ids.includes(x)));
      return { prevList, prevCounts };
    },
    onError: (e, _v, ctx) => {
      if (ctx?.prevList) qc.setQueryData(key, ctx.prevList);
      if (ctx?.prevCounts) qc.setQueryData(countsKey, ctx.prevCounts);
      showToast({ message: errorText(t, e) });
    },
    onSuccess: (r, v) => {
      qc.setQueryData(countsKey, r.counts);
      const first = items.find((n) => n.id === v.ids[0]);
      const word = v.status === "approved" ? "approved" : "hidden";
      showToast({
        message:
          v.ids.length === 1 && first
            ? t(`inbox.toast.${word}One`, { name: noteName(first, t("inbox.anonymous")) })
            : t(`inbox.toast.${word}Many`, { count: v.ids.length }),
      });
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ["notes", bookId] }),
  });

  // ── delete with a client-side Undo ──
  // The Undo toast is shown for as long as `pending` is not empty, whatever other toast comes and goes meanwhile.
  function startDelete(ids: string[]) {
    const all = [...pending, ...items.filter((n) => ids.includes(n.id))];
    setPending(all);
    setSel((s) => s.filter((x) => !ids.includes(x)));
    setConfirm(null);
    window.clearTimeout(delTimer.current);
    delTimer.current = window.setTimeout(() => void sendDelete(all), UNDO_MS);
  }
  function undo() {
    if (inflight.current.size > 0) return; // too late: the request is on its way
    window.clearTimeout(delTimer.current);
    setPending([]);
    void qc.invalidateQueries({ queryKey: ["notes", bookId] }); // `move` may have dropped them from the cache
  }
  async function sendDelete(all: NoteItem[]) {
    const ids = all.map((n) => n.id).filter((id) => !inflight.current.has(id));
    if (ids.length === 0) return;
    ids.forEach((id) => inflight.current.add(id));
    try {
      await order.flush();
      const c = await deleteNotes(bookId, ids);
      qc.setQueryData(countsKey, c);
      const drop = new Set(ids);
      qc.setQueriesData<InfiniteData<NotePage>>(
        { queryKey: ["notes", bookId] },
        (d) =>
          d && {
            ...d,
            pages: d.pages.map((p) => ({ ...p, items: p.items.filter((n) => !drop.has(n.id)) })),
          },
      );
    } catch (e) {
      showToast({ message: errorText(t, e) });
    } finally {
      ids.forEach((id) => inflight.current.delete(id));
      setPending((p) => p.filter((n) => !ids.includes(n.id)));
      void qc.invalidateQueries({ queryKey: ["notes", bookId] });
    }
  }
  function askDelete(ids: string[]) {
    const one = items.find((n) => n.id === ids[0]);
    if (ids.length > 1 || one?.status === "approved") setConfirm(ids);
    else startDelete(ids);
  }

  function move(from: number, to: number) {
    if (from === to || to < 0 || to >= items.length) return;
    const next = arrayMove(items, from, to).map((n, i) => ({ ...n, position: i + 1 }));
    qc.setQueryData<InfiniteData<NotePage>>(
      key,
      (d) =>
        d && { pageParams: [undefined], pages: [{ ...d.pages[0]!, items: next, next_id: "" }] },
    );
    const moved = next[to]!;
    setAnnounce(
      t("inbox.moved", {
        name: noteName(moved, t("inbox.anonymous")),
        position: to + 1,
        total: next.length,
      }),
    );
    // a note in its Undo window is still approved on the server, so it belongs to the set
    order.schedule([...next, ...pending.filter((n) => n.status === "approved")].map((n) => n.id));
  }

  useEffect(
    () => () => {
      window.clearTimeout(toastTimer.current);
      window.clearTimeout(delTimer.current); // leaving the page drops a delete in its Undo window
    },
    [],
  );

  const canOrder = filter === "approved" && !hasNextPage && !list.isError;
  const orderFor = (n: NoteItem, i: number) =>
    canOrder
      ? {
          position: i + 1,
          total: items.length,
          dragging: drag,
          onMove: (to: number) => move(i, to),
          onDragStart: () => setDrag({ id: n.id, over: n.id }),
          onDragOver: (over: string) => setDrag((d) => (d && d.over !== over ? { ...d, over } : d)),
          onDragEnd: (commit: boolean) => {
            if (commit && drag && drag.over !== drag.id)
              move(
                items.findIndex((x) => x.id === drag.id),
                items.findIndex((x) => x.id === drag.over),
              );
            setDrag(null);
          },
          indexOf: (id: string) => items.findIndex((x) => x.id === id),
        }
      : undefined;

  const pick = (f: Filter) => {
    setFilter(f);
    setSel([]);
    setOrdering(false);
  };
  const toggle = (id: string) =>
    setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  const selected = sel.filter((id) => items.some((n) => n.id === id));
  const confirmNote =
    confirm && confirm.length === 1 ? items.find((n) => n.id === confirm[0]) : undefined;
  const all = selected.length > 0 && selected.length === Math.min(items.length, BATCH_MAX);
  const orderMode = ordering && !wide && canOrder;

  const pageError: unknown = list.error; // read outside the narrowing of `list.isSuccess` below
  const offline = list.error instanceof ApiError && list.error.status === 0;
  return (
    <>
      <header className="inbox-head">
        <div>
          <h1>{t("inbox.title")}</h1>
          {counts && (
            <p className="inbox-summary">
              {counts.pending
                ? t("inbox.summary", { pending: counts.pending, approved: counts.approved })
                : t("inbox.summaryClear", { approved: counts.approved })}
            </p>
          )}
        </div>
        <Link to={`/yearbooks/${bookId}/note-links`} className={buttonClass("secondary")}>
          {t("inbox.getLink")}
        </Link>
      </header>
      {offline && (
        <Banner variant="offline">
          <strong>{t("inbox.offline")}</strong> {t("inbox.offlineBody")}
        </Banner>
      )}

      <div role="group" aria-label={t("inbox.filter")} className="inbox-tabs">
        {FILTERS.map((f) => (
          <button
            key={f}
            type="button"
            aria-pressed={filter === f}
            onClick={() => pick(f)}
            className={filter === f ? "is-active" : undefined}
          >
            {t(`inbox.tab.${f}`)}
            {counts && (
              <>
                {" "}
                <span className="inbox-count">{counts[f]}</span>
              </>
            )}
          </button>
        ))}
      </div>

      {list.isPending && (
        <div role="status" aria-label={t("auth.loading")}>
          {[0, 1, 2].map((i) => (
            <div key={i} className="inbox-card">
              <Skeleton width="40%" height={20} />
              <Skeleton height={48} />
            </div>
          ))}
        </div>
      )}
      {list.isError && (
        <EmptyState
          title={t("inbox.loadError")}
          action={<Button onClick={() => void list.refetch()}>{t("auth.retry")}</Button>}
        >
          {offline ? t("inbox.loadErrorBody") : errorText(t, list.error)}
        </EmptyState>
      )}

      {list.isSuccess && selected.length > 0 && (
        <div className="inbox-bulk" role="region" aria-label={t("inbox.bulk")}>
          <strong>{t("inbox.selected", { count: selected.length })}</strong>
          <Button
            variant="secondary"
            onClick={() => change.mutate({ ids: selected, status: "approved" })}
          >
            {t("inbox.approve")}
          </Button>
          <Button
            variant="secondary"
            onClick={() => change.mutate({ ids: selected, status: "hidden" })}
          >
            {t("inbox.hide")}
          </Button>
          <Button variant="danger" onClick={() => askDelete(selected)}>
            {t("inbox.delete")}
          </Button>
          <Button variant="ghost" onClick={() => setSel([])}>
            {t("inbox.clear")}
          </Button>
        </div>
      )}

      {list.isSuccess && items.length === 0 && counts?.all === 0 && filter === "all" && (
        <EmptyState
          title={t("inbox.firstRun.title")}
          action={
            <Link to={`/yearbooks/${bookId}/note-links`} className={buttonClass("primary")}>
              {t("inbox.firstRun.cta")}
            </Link>
          }
        >
          {t("inbox.firstRun.body")}
        </EmptyState>
      )}
      {list.isSuccess && items.length === 0 && !(counts?.all === 0 && filter === "all") && (
        <EmptyState title={t(`inbox.empty.${filter}.title`)}>
          {t(`inbox.empty.${filter}.body`)}
        </EmptyState>
      )}

      {list.isSuccess && items.length > 0 && (
        <>
          <div className="inbox-tools">
            <label className="inbox-selectall">
              <input
                type="checkbox"
                checked={all}
                onChange={() =>
                  setSel(
                    all ? [] : items.slice(0, BATCH_MAX).map((n) => n.id), // the server takes 100 at a time
                  )
                }
              />
              {t("inbox.selectAll")}
            </label>
            {canOrder && !wide && !orderMode && (
              <Button variant="secondary" onClick={() => setOrdering(true)}>
                {t("inbox.bookOrder")}
              </Button>
            )}
          </div>
          {filter === "approved" && (
            <p className="inbox-hint">{t(wide ? "inbox.hintWide" : "inbox.hintNarrow")}</p>
          )}
          {orderMode ? (
            <>
              <Button variant="primary" onClick={() => setOrdering(false)}>
                {t("inbox.done")}
              </Button>
              <ol className="order-list">
                {items.map((n, i) => (
                  <OrderRow key={n.id} note={n} order={orderFor(n, i)!} />
                ))}
              </ol>
            </>
          ) : (
            <ul className="inbox-list">
              {items.map((n, i) => (
                <NoteCard
                  key={n.id}
                  note={n}
                  lang={i18n.language}
                  selected={selected.includes(n.id)}
                  onSelect={() => toggle(n.id)}
                  onStatus={(status) => change.mutate({ ids: [n.id], status })}
                  onDelete={() => askDelete([n.id])}
                  {...(orderFor(n, i) ? { order: orderFor(n, i)! } : {})}
                />
              ))}
            </ul>
          )}
          {hasNextPage && filter !== "approved" && (
            <Button busy={isFetchingNextPage} onClick={() => void fetchNextPage()}>
              {t("inbox.more")}
            </Button>
          )}
          {isFetchNextPageError && (
            <p role="alert" className="error">
              {errorText(t, pageError)}
            </p>
          )}
        </>
      )}

      <div className="visually-hidden" role="status" aria-live="polite">
        {announce}
      </div>
      {(pending.length > 0 || toast) && (
        <div className="inbox-toast">
          {toast && <Toast message={toast.message} />}
          {pending.length > 0 && (
            <Toast
              message={
                pending.length === 1
                  ? t("inbox.toast.deletedOne")
                  : t("inbox.toast.deletedMany", { count: pending.length })
              }
              actionLabel={t("inbox.undo")}
              onAction={undo}
            />
          )}
        </div>
      )}
      {confirm && (
        <Dialog
          title={
            confirm.length === 1
              ? t("inbox.confirm.titleOne")
              : t("inbox.confirm.titleMany", { count: confirm.length })
          }
          confirmLabel={t("inbox.delete")}
          cancelLabel={t("inbox.cancel")}
          danger
          onConfirm={() => startDelete(confirm)}
          onCancel={() => setConfirm(null)}
        >
          <p>{confirmNote ? t("inbox.confirm.bodyOne") : t("inbox.confirm.bodyMany")}</p>
          <Button
            variant="secondary"
            onClick={() => {
              change.mutate({ ids: confirm, status: "hidden" });
              setConfirm(null);
            }}
          >
            {t("inbox.hideInstead")}
          </Button>
        </Dialog>
      )}
    </>
  );
}
