# Layout storage budget and abuse limits (E09 stage 2, T-179)

What the free-layout editor lets one user store, measured on the real tables, what it costs, which limit stops which abuse, and the query an
operator runs to find the biggest users. The numbers come from `TestMaximalUserStorage`
(`go test -tags integration -run TestMaximalUserStorage -v ./internal/booklayout`, MySQL 8.4, `utf8mb4`, InnoDB defaults); run it again after any change of a
limit. Threat model: [stage2-threat-model.md](stage2-threat-model.md). Format and limits: [templates.md](templates.md).

## 1. The limits that bound the growth

| What | Limit | Where |
|---|---|---|
| Current layout of a book | 128 KiB (131 072 bytes) as sent **and** as stored (canonical form; NFC can make a text longer in bytes, so the stored form is checked too) | `Validate`, `MaxBytes` |
| Items | 40 per page, 150 per document, 500 characters per text | `validate.go` |
| Nesting of a document | 16 levels (the real maximum is 5), checked before anything is decoded | `tooDeep`, `MaxDepth` |
| Snapshots per book | 30 (newest kept, pruned in the writing transaction); an automatic one only after 10 minutes, a reset, restore or apply always | `snapshotCurrent` |
| Own templates per user | 20, each at most 64 KiB (`MaxTemplateBytes`), made under the user's row lock | `createTemplate` |
| Books per user | **20** (`maxBooksOwned`, `internal/yearbook/validate.go`) | the only cap on how many books can hold a layout |
| Request bodies | `save` 132 KiB (`MaxBytes` + 4 KiB), every other write route 8 KiB, everything else the router's 1 MiB | `handler.go` |

## 2. Worst case, by the limits (payload bytes, before MySQL overhead)

| | Bytes | MiB |
|---|---|---|
| One book: 1 layout + 30 snapshots, 31 x 128 KiB | 4 063 232 | 3.88 |
| 20 own templates, 20 x 64 KiB | 1 310 720 | 1.25 |
| One user: 20 books + 20 templates | 82 575 360 | **78.75** |

## 3. Measured on a real table

One book with a maximal layout (150 text items, 123 098 bytes stored, a different document each time), 31 saves so that 30 snapshots exist, and 20 own
templates of 62 731 bytes. `LENGTH(layout_json)` is the payload; `data_length` and `index_length` are read from `information_schema.tables` after
`ANALYZE TABLE` and include the overflow pages MySQL keeps for the MEDIUMTEXT values.

| Table | Rows | Payload bytes | `data_length` | `index_length` | On disk / payload |
|---|---|---|---|---|---|
| `book_layout_tab` | 1 | 123 098 | 540 672 | 16 384 | 4.5 (see below) |
| `book_layout_version_tab` | 30 | 3 691 440 | 4 669 440 | 49 152 | 1.28 |
| `book_template_tab` | 20 | 1 254 620 | 1 589 248 | 32 768 | 1.29 |
| **One maximal book + 20 templates** | 51 | **5 069 158** | | | **6 897 664 bytes (6.58 MiB)**, 1.36 |

The current-layout row shows 540 672 bytes for 123 098 because it was rewritten 31 times in a few seconds and MySQL had not yet purged the old versions of
the overflow pages; it settles back as the purge runs. Counting the settled figure, a maximal book is **5 275 648 bytes on disk (5.03 MiB)** and a maximal user,
20 such books and 20 templates, is about **107 MB (102 MiB)** on disk, 1.3 times the payload of 78.75 MiB.

## 4. Cost

Basis: the leader's estimate in [cost-estimate.md](cost-estimate.md), RDS MySQL on gp3 storage, about **0.14 USD per GB-month** in Singapore (single
availability zone; double for Multi-AZ; plus ±20 % on the leader's price list; backups up to the database size are free).

| Per 1 000 students | On disk | Storage per month, single-AZ | Multi-AZ |
|---|---|---|---|
| **Worst case**, every student at the maximum (20 books, 30 snapshots each, 20 templates) | 107 GB | about 15 USD | about 30 USD |
| **Typical** (leader's figure: 5 saves a week, 3 KiB layouts; one book; a snapshot per save once 10 minutes apart; 30 reached after six weeks, so 31 x 3 KiB of payload; two 3 KiB templates), about 115 KiB on disk per student | 0.11 GB | about 0.02 USD | about 0.03 USD |

The typical on-disk figure is an estimate (payload x 1.2 for page fill and the two unique indexes), not a measurement. The worst case alone is 5 times the
20 GB volume of the lean estimate: it takes a deliberate abuser, not normal use, so it is a **monitoring and cap** question, not a pricing one.

## 5. Recommendation (needs the leader's answer; behaviour is unchanged)

The worst case per user, **78.75 MiB payload (107 MB on disk), is above the 30 MB threshold** of the spec. Nothing stops one account from filling it: free
accounts, 20 books, and `apply-template`, `restore` and `reset` store a snapshot on every call, so the 30 snapshots of a book fill in about a
minute of calls inside the 60 per minute limit, and 20 books take about 11 minutes. Options, cheapest first:

1. **A total cap on snapshots per user**, say 60 across all books (newest kept, 7.5 MiB at 128 KiB): the pruning statement already runs in the writing
   transaction, it would count per owner instead of per book. Worst case per user becomes 20 x 128 KiB + 60 x 128 KiB + 1.25 MiB = about 11.3 MiB.
2. Keep 30 per book but lower the books that may hold a **layout** to, say, 5 (the cap on books stays 20): worst case about 20 MiB.
3. Do nothing and watch the operator query below; act when one user passes 25 MB.

## 6. Operator query: the biggest users

Layout bytes held per user (current layouts, snapshots, own templates), largest first. `TestMaximalUserStorage` runs this text and checks that this
document carries it verbatim. For the size of the tables themselves: `SELECT table_name, data_length, index_length FROM information_schema.tables WHERE
table_schema = DATABASE() AND table_name IN ('book_layout_tab','book_layout_version_tab','book_template_tab')`.

```sql
SELECT owner_id, SUM(bytes) AS total_bytes, COUNT(*) AS rows_held FROM (
  SELECT y.owner_id, LENGTH(l.layout_json) AS bytes FROM book_layout_tab l JOIN yearbook_tab y ON y.id = l.yearbook_id
  UNION ALL SELECT y.owner_id, v.bytes FROM book_layout_version_tab v JOIN yearbook_tab y ON y.id = v.yearbook_id
  UNION ALL SELECT t.owner_id, t.bytes FROM book_template_tab t
) held GROUP BY owner_id ORDER BY total_bytes DESC LIMIT 20
```

## 7. Rate limits and body caps per route

All routes are POST (get is GET), owner only, behind the origin guard and `RequireUser`: an anonymous call is **401 before a byte of the body is
read**. The per-user limits share two buckets (key: the internal user id), `book_layout` (`SMEM_RATE_LAYOUT_SAVE_PER_MINUTE`, default 60 per minute) and
`book_layout_burst` (20 per 10 seconds); a refused call is `429 ERROR_RATE_LIMITED` with `Retry-After`, and both fail open when Redis is down
([redis.md](redis.md)). The test `TestRoutesRefuseAnonymousOversizeAndTooMany` proves each row.

| Route | Per-user limit | Body cap (413 `ERROR_TOO_LARGE`, before decoding) |
|---|---|---|
| `GET get` | **none** | no body |
| `POST save` | burst 20 / 10 s and per minute | 132 KiB, nesting and size checked before the document is decoded |
| `POST reset` | **none** (cannot amplify: it deletes the layout, so the next one has nothing to snapshot, and a layout needs a limited save) | 8 KiB |
| `POST version-list`, `version-get`, `restore` | burst and per minute | 8 KiB |
| `POST template-list`, `template-create`, `template-delete`, `apply-template` | burst and per minute | 8 KiB |

Findings of this task: the 8 KiB caps are new (these routes used the router's 1 MiB before); `get` and `reset` take no limit, which is the T-153 design
("reads and reset are not saves") and is left as it is, but `get` is a database read of up to 128 KiB that a user can repeat without bound: see the board
comment on T-179.
