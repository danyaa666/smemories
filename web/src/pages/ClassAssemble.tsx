import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { deleteClassPage, getClassBook, updateOrder, type ClassPage } from "../api/classes";
import { ApiError } from "../api/client";
import { errorText } from "../auth/errors";
import { AdminOnly } from "../classes/AdminOnly";
import { classErrorText } from "../classes/classErrors";
import { OrderList, type OrderItem } from "../classes/OrderList";
import { PageEditor } from "../classes/PageEditor";
import { orderOf, orderPayload, reconcile, sameOrder, type BookOrder } from "../classes/ordering";
import { Banner } from "../components/ui/Banner";
import { Button, buttonClass } from "../components/ui/Button";
import { Dialog } from "../components/ui/Dialog";

/** /classes/:id/assemble. Admins only: a student (or a stranger) gets the same not-found screen as any unknown address. */
export function ClassAssemble() {
  return <AdminOnly>{(classId) => <Assemble classId={classId} />}</AdminOnly>;
}

function Assemble({ classId }: { classId: string }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const key = ["class-book", classId];
  const book = useQuery({ queryKey: key, queryFn: () => getClassBook(classId) });
  const [local, setLocal] = useState<BookOrder | null>(null); // the admin's arrangement, null while it equals the server's
  const [stale, setStale] = useState(false);
  const [message, setMessage] = useState("");
  const [editing, setEditing] = useState<ClassPage | "new" | null>(null);
  const [deleting, setDeleting] = useState<ClassPage | null>(null);
  const reload = () => qc.invalidateQueries({ queryKey: key });

  const save = useMutation({
    mutationFn: (o: BookOrder) => updateOrder(orderPayload(classId, o)),
    onSuccess: async () => {
      await reload();
      setLocal(null);
      setMessage(t("assemble.saved"));
    },
    onError: (e) => {
      if (!(e instanceof ApiError && (e.status === 400 || e.status === 409))) return;
      // the lists are stale (someone joined, a page was added or deleted): take the server's order and say so
      setLocal(null);
      setStale(true);
      void reload();
    },
  });
  const remove = useMutation({
    mutationFn: (p: ClassPage) => deleteClassPage(classId, p.id),
    onSuccess: async (_r, p) => {
      setDeleting(null);
      await reload();
      setMessage(t("assemble.deleted", { name: p.title }));
    },
  });

  if (book.isPending) return <p role="status">{t("auth.loading")}</p>;
  if (book.isError)
    return (
      <>
        <p role="alert" className="error">
          {errorText(t, book.error)}
        </p>
        <Button onClick={() => void book.refetch()}>{t("auth.retry")}</Button>
      </>
    );
  const data = book.data;
  const server = orderOf(data);
  const order = reconcile(local, server);
  const dirty = !sameOrder(order, server);
  const change = (patch: Partial<BookOrder>) => {
    setLocal({ ...order, ...patch });
    setStale(false);
    save.reset();
  };
  const pages = new Map([...data.front_pages, ...data.back_pages].map((p) => [p.id, p]));
  const pageItems = (ids: string[]): OrderItem[] =>
    ids.flatMap((pid) => {
      const p = pages.get(pid);
      if (!p) return [];
      return [
        {
          id: p.id,
          name: p.title,
          meta: t(`assemble.kind.${p.kind}`),
          body: p.body,
          extra: (
            <div className="row-actions assemble-actions">
              <Button
                onClick={() => {
                  setEditing(p);
                  setMessage("");
                }}
                aria-label={t("assemble.editFor", { name: p.title })}
              >
                {t("assemble.edit")}
              </Button>
              <Button
                variant="danger"
                onClick={() => {
                  remove.reset();
                  setDeleting(p);
                }}
                aria-label={t("assemble.deleteFor", { name: p.title })}
              >
                {t("assemble.delete")}
              </Button>
            </div>
          ),
        },
      ];
    });
  const members = new Map(data.members.map((m) => [m.member_id, m]));
  const memberItems: OrderItem[] = order.members.flatMap((mid) => {
    const m = members.get(mid);
    if (!m) return [];
    return [
      {
        id: m.member_id,
        name: m.name,
        meta: m.linked
          ? t("classes.page.notes", { n: m.approved_total }) +
            (m.truncated ? ` · ${t("assemble.truncated", { n: m.note_count })}` : "")
          : t("assemble.noPage"),
      },
    ];
  });
  const conflict =
    save.error instanceof ApiError && (save.error.status === 400 || save.error.status === 409);

  return (
    <>
      <p>
        <Link to={`/classes/${classId}`} className="back-link">
          {t("assemble.toClass")}
        </Link>
      </p>
      <h1>{t("assemble.title", { name: data.class.name })}</h1>
      <p>{t("assemble.help")}</p>
      {stale && <Banner variant="error">{t("assemble.stale")}</Banner>}
      {save.error && !conflict && (
        <p role="alert" className="error">
          {classErrorText(t, save.error, "assemble")}
        </p>
      )}
      <div className="row-actions">
        <Button
          variant="primary"
          disabled={!dirty}
          busy={save.isPending}
          onClick={() => save.mutate(order)}
        >
          {t("assemble.save")}
        </Button>
        <Button disabled={!dirty || save.isPending} onClick={() => setLocal(null)}>
          {t("assemble.discard")}
        </Button>
        <Button
          onClick={() => {
            setEditing("new");
            setMessage("");
          }}
          disabled={editing === "new"}
        >
          {t("assemble.add")}
        </Button>
        <Link to={`/classes/${classId}/book`} className={buttonClass()}>
          {t("classes.print")}
        </Link>
      </div>
      {dirty && <p role="status">{t("assemble.unsaved")}</p>}
      {editing && (
        <PageEditor
          key={editing === "new" ? "new" : editing.id}
          classId={classId}
          page={editing === "new" ? null : editing}
          onCancel={() => setEditing(null)}
          onDone={async (p) => {
            setEditing(null);
            await reload();
            setMessage(t("assemble.pageSaved", { name: p.title }));
          }}
        />
      )}
      <section aria-labelledby="front-pages">
        <h2 id="front-pages">{t("assemble.front")}</h2>
        {order.front.length === 0 && <p>{t("assemble.noPages")}</p>}
        <OrderList
          labelledBy="front-pages"
          items={pageItems(order.front)}
          onChange={(front) => change({ front })}
          announce={setMessage}
        />
      </section>
      <section aria-labelledby="students">
        <h2 id="students">{t("assemble.students")}</h2>
        {memberItems.length === 0 && <p>{t("assemble.noStudents")}</p>}
        <OrderList
          labelledBy="students"
          items={memberItems}
          onChange={(members) => change({ members })}
          announce={setMessage}
        />
      </section>
      <section aria-labelledby="back-pages">
        <h2 id="back-pages">{t("assemble.back")}</h2>
        {order.back.length === 0 && <p>{t("assemble.noPages")}</p>}
        <OrderList
          labelledBy="back-pages"
          items={pageItems(order.back)}
          onChange={(back) => change({ back })}
          announce={setMessage}
        />
      </section>
      <div className="visually-hidden" role="status" aria-live="polite">
        {message}
      </div>
      {deleting && (
        <Dialog
          title={t("assemble.deleteTitle", { name: deleting.title })}
          confirmLabel={t("assemble.delete")}
          cancelLabel={t("classes.cancel")}
          danger
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
          onCancel={() => setDeleting(null)}
        >
          <p>{t("assemble.deleteBody")}</p>
          {remove.error && (
            <p role="alert" className="error">
              {classErrorText(t, remove.error, "assemble")}
            </p>
          )}
        </Dialog>
      )}
    </>
  );
}
