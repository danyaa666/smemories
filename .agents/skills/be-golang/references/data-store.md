# Data Store Integration

> Schema design delegated to specialized skills. This section covers Go-side conventions only.

## 1. GORM (Relational DB) via Adapter → `@be-rldb`

- Use `gorm:"primaryKey"`, `gorm:"autoCreateTime"`, `gorm:"autoUpdateTime"` tags.
- Use `gorm:"serializer:json"` for complex/JSON columns.
- Use GORM Lifecycle Hooks (`BeforeCreate`, etc.) for pre-persist logic.
- Keep transactions short. No single-row DB ops in loops — batch instead.
- Chunk large writes: 2000–5000 records per batch.
- Avoid DB joins — prefer application-layer joins.

## 2. ClickHouse via Adapter → `@be-clickhouse`

- Client: `github.com/ClickHouse/clickhouse-go/v2/lib/driver`.
- Always batch: `PrepareBatch` → `batch.AppendStruct()` → `batch.Send()`.
- Declare insert queries as named constants.

## 3. Elasticsearch via Adapter → `@be-es`

- Client: `github.com/elastic/go-elasticsearch/v8/typedapi/*` (typed API).
- Pass connections as `conn *elasticsearch.TypedClient`.
