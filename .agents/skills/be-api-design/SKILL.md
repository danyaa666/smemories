---
name: be-api-design
description: >
  API design standards for HTTP REST and gRPC/Protobuf. Use when designing or reviewing
  API contracts in design docs or implementation. Covers path naming, response envelope,
  error codes, pagination, authentication, proto conventions, and contract documentation rules.
---

# API Design Standards

**Principle:** Every contract must be self-documenting — FE/client engineers implement without follow-up questions.

---

## HTTP vs gRPC

| Criterion | HTTP (REST JSON) | gRPC |
|---|---|---|
| Consumer | Browser / mobile | Internal service-to-service |
| Payload | JSON | Protobuf |
| Streaming | No | Bidirectional |

---

# Part 1 — HTTP

## URL Paths

Format: `/api/<namespace>/<action>` or `/api/<namespace>/<action>-<resource>`

Rules: kebab-case, singular resources, `GET` for reads, `POST` for mutations, flat structure (no deep nesting).

```
GET  /api/staff/get            GET  /api/staff/get-list
POST /api/customer/create      POST /api/auth/verify-token
POST /api/auth/setup-2fa  ✓    POST /api/auth/2fa/setup  ✗
POST /api/staff/batch-update
```

Verbs: `get`, `list`, `create`, `update`, `delete`, `verify`, `setup`. Batch: `batch-<action>`.

## Response Envelope

**Success:**
```json
{ "status": "OK", "data": { ...payload... } }
```
**Error:**
```json
{ "status": "ERROR_CODE_STRING", "error_message": "Human readable description" }
```

`status` always string. `"OK"` = success. Anything else = error code (`SCREAMING_SNAKE_CASE`).

## Error Codes
- List only **business-specific** errors per endpoint.
- Do NOT document `error_internal`, `error_unauthorized`, `error_param` — globally implied.

## Pagination — Keyset

Request: `limit` (int), `next_id` (string, omit on first page).
Response: `next_id` (string, first ID of next page; empty = no more pages).

```json
{ "status": "OK", "data": { "items": [...], "next_id": "1048576" } }
```

## Endpoint Documentation Format

```markdown
### <Action> <Resource>
**Method:** GET | POST  **Path:** /api/...  **Auth:** Required | Public | Optional

**Request**
| Field | Type | Required | Description |
|---|---|---|---|

**Response**
| Field | Type | Description |
|---|---|---|

**Error Codes**
| Code | Description |
|---|---|

**Example Request** ```json ... ```
**Example Response** ```json ... ```
```

---

# Part 2 — gRPC / Protobuf

## RPC Naming
PascalCase `<Action><Resource>`: `GetStaff`, `ListStaff`, `CreateOrder`, `VerifyToken`.

## Authentication
Every RPC declares auth explicitly:
```protobuf
option (permission.auth) = AUTH_REQUIREMENT_REQUIRED;
```

| Value | Meaning |
|---|---|
| `AUTH_REQUIREMENT_REQUIRED` | Must be authenticated (default for private) |
| `AUTH_REQUIREMENT_PUBLIC` | No auth |
| `AUTH_REQUIREMENT_OPTIONAL` | Auth used if present, not enforced |

## Error Codes
- Document only **business-specific** codes per RPC via `(apidoc.errors)`.
- Do NOT document `error_internal`, `error_unauthorized`, `error_param`.

## Proto Plugins (Golang)

### protoc-gen-validate — Field Validation
Declare validation constraints directly in `.proto` message fields.

```protobuf
import "validate/validate.proto";

message CreateOrderRequest {
  int64  user_id  = 1 [(validate.rules).int64.gt  = 0];
  int32  quantity = 2 [(validate.rules).int32.gte = 1];
  string currency = 3 [(validate.rules).string    = {min_len: 3, max_len: 3}];
}
```

- Prefer proto-level validation over ad-hoc checks in application code.
- Declare only the minimal constraint that prevents invalid data (no over-constraining).
- Go call pattern and error mapping → `@be-golang`.

### protoc-gen-openapiv2 — Swagger Documentation
Annotate every RPC with `openapiv2_operation` to drive the generated `swagger.json`.

```protobuf
import "protoc-gen-openapiv2/options/annotations.proto";

rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse) {
  option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
    summary:     "Create a new order";
    description: "Places an order for the authenticated user.";
    tags:        ["Orders"];
  };
}
```

> Plugin configuration (`buf.gen.yaml`) → `@be-golang`.

## Proto Documentation Format

```markdown
### <RpcName>
**Auth:** AUTH_REQUIREMENT_...  **Description:** One sentence.

```protobuf
rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse) {
  option (permission.auth) = AUTH_REQUIREMENT_REQUIRED;
  option (apidoc.errors) = {
    error_codes: { code: "error_balance_insufficient" description: "Insufficient funds" }
  };
}
message CreateOrderRequest {
  int64 user_id = 1; // User placing order
  int64 item_id = 2; // Item ordered
  int32 quantity = 3; // >= 1
}
message CreateOrderResponse {
  int64 order_id = 1;
  string status = 2; // "pending"
  int64 created_at = 3; // Unix ms
}
```

**Error Codes**
| Code | Description |
|---|---|
```

---

# Part 3 — Contract Documentation Rules

## Required per Endpoint/RPC
1. **Purpose** — one sentence, who calls it.
2. **Auth** — explicit.
3. **Request fields** — name, type, required, constraints, meaning.
4. **Response fields** — name, type, meaning.
5. **Business error codes** — with descriptions.
6. **Example** — happy-path request + response.
7. **Integration notes** — (when needed) edge cases, retry/polling behaviour.

## Field Rules
- Timestamps: Unix ms as `int64` — always state in description.
- IDs and `int64`/`uint64` numbers: use `int64` in storage and proto; **always return as `string` in JSON responses** to prevent JavaScript 64-bit precision loss (`Number.MAX_SAFE_INTEGER = 2^53 - 1`).
- Enums: list all valid values with meanings.
- Required/optional: stated for every field.
- Defaults: stated if they affect behaviour.

## Exclude
- Internal implementation details (Go structs, DB columns).
- Redundant global error codes.
- Vague descriptions ("the id").

---

## Checklist

- [ ] Every endpoint/RPC has purpose statement
- [ ] Auth explicitly stated
- [ ] All request fields documented (name, type, required, constraints)
- [ ] All response fields documented (name, type, meaning)
- [ ] All business error codes listed
- [ ] Happy-path example provided
- [ ] Timestamps stated as Unix ms int64
- [ ] int64/uint64 response fields documented as string type
- [ ] Enum values listed
- [ ] Keyset pagination (if applicable)
- [ ] HTTP: kebab-case flat paths
- [ ] gRPC: PascalCase `<Action><Resource>`
- [ ] No global errors per-endpoint
