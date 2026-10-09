# Proto — Go Implementation

Contract design, field annotations, and RPC naming → `@be-api-design`.

**Rules (Go side):**
- Every RPC declares `(permission.auth)` explicitly.
- Every RPC lists business error codes via `(apidoc.errors)`.
- Do NOT document `error_internal`, `error_unauthorized`, `error_param` per-RPC.
- Keyset pagination: `limit` + `next_id`. No offset pagination.
- Use proto-generated enum types for all status/type/category fields shared with the API — do not declare parallel Go enums.

## protoc-gen-validate

Proto annotation syntax → `@be-api-design`.

- Call `req.Validate()` at the top of every RPC handler; return `errors.ErrorParam` on failure.
- Do **not** duplicate proto-level validation in Go business logic.
```go
func (c *OrderController) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
    if err := req.Validate(); err != nil {
        return nil, errors.WithErrorCode(errors.ErrorParam, err)
    }
    // ...
}
```

## protoc-gen-openapiv2

Proto annotation syntax → `@be-api-design`.

- Annotate every RPC with `openapiv2_operation` (`summary`, `description`, `tags`).
- The generated `swagger.json` is the source of truth for HTTP/JSON clients.
- Keep annotations in sync with any contract changes.

**`buf.gen.yaml` plugin entries:**
```yaml
plugins:
  - plugin: buf.build/bufbuild/validate-go         # protoc-gen-validate
    out: generated
    opt: paths=source_relative
  - plugin: buf.build/grpc-ecosystem/gateway/v2    # protoc-gen-openapiv2
    out: generated
    opt: paths=source_relative
```

## JSON Serialization of 64-bit Integers

`int64` and `uint64` fields **must** be serialized as JSON strings to prevent JavaScript precision loss (`Number.MAX_SAFE_INTEGER = 2^53 - 1`). Apply this to all IDs, snowflake IDs, and any other `int64`/`uint64` response fields.

Use the `string` struct tag on JSON-serialized response structs:

```go
type OrderResponse struct {
    ID        int64  `json:"id,string"`         // serializes as "123456789"
    UserID    int64  `json:"user_id,string"`
    Amount    int64  `json:"amount"`             // int32-range values may omit ,string
    CreatedAt int64  `json:"created_at"`         // Unix ms timestamp — int64 but stays numeric; document clearly
}
```

- All IDs and snowflake fields: always `json:",string"`.
- Timestamps (Unix ms): stay as numeric `int64` — document the unit explicitly in API contracts.
- Proto `int64` fields: when served over HTTP/JSON via grpc-gateway, annotate with `jstype = JS_STRING` in the proto field option.
