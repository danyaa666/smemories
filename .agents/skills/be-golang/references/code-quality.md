# Code Quality

**Style:** Follow [Effective Go](https://go.dev/doc/effective_go). Small, readable functions. Prefer explicit, readable code and small focused functions. Start with correct, working code; optimize afterward based on data.

**Formatting and linting:** Enforce `gofmt` and `golangci-lint` compliance before completion.

**No unused code:** Do not generate unused functions, variables, types, imports, or constants. Every declaration must be referenced. Remove dead code immediately.

**No hardcode:** All configuration values, thresholds, limits, and identifiers must be declared as named constants or injected via config. Inline literals are forbidden in business logic.

**Type inference:** Let the compiler infer types when the right-hand side makes it unambiguous. Do not repeat the type on the left-hand side.
```go
// Correct
a := 1
result := computeTotal(order)

// Wrong
var a int = 1
var result *Order = computeTotal(order)
```
Exceptions: interface-typed variables, nil-initialized slices/maps, and cases where inference would produce an unintended type.

**Proto enums:** For any enum shared with the API (status, type, category, etc.), use the generated proto enum type directly. Do not re-declare a parallel Go `iota` enum.
```go
// Correct — use proto-generated enum
status := pb.OrderStatus_ORDER_STATUS_PENDING

// Wrong — duplicate enum
type OrderStatus int
const (
    OrderStatusPending OrderStatus = iota
)
```

**Pointers by default:** Use pointers for struct values unless there is a specific, explicit reason not to.
- Method receivers: default to pointer receivers for structs.
- Function params/returns: default to `*Struct` instead of `Struct`.
- Struct fields and dependencies: default to pointer concrete implementations.
- DI note: constructors may return interfaces per Section 4, but implementations behind those interfaces should be pointer types.
- Do not use pointers for interface types (`*MyInterface` is prohibited).
- Allowed value-type exceptions:
  - tiny immutable value objects where copy semantics are intentional,
  - scalar aliases/enums and fixed-size data where value semantics are clearer,
  - map/slice/channel/function types (already reference-like),
  - interoperability cases that require value types.
- If you choose a non-pointer struct value where pointer is expected, add a short comment explaining why.

**Interface design discipline:** 
- Keep interfaces small and behavior-focused.
- Define interfaces at the consumer side when possible.
- Avoid introducing interfaces when a concrete type is sufficient.

**Constants:** No magic numbers. Define in `internal/consts/defaults.go` or module-local. `//nolint:mnd` prohibited (except crypto constants).

**Security (Gosec):** Suppress G115 only with `// #nosec G115 // reason`. Validate/sanitize all external inputs.

**Context:** Use keys from `internal/platform/appctx`. Pass `ctx context.Context` through controller → manager → adapter. Never store `context.Context` in structs.

**Concurrency:** Ensure code is safe under concurrent execution. Every goroutine must have cancellation or bounded lifecycle. Prefer channels for orchestration and mutex/atomic for shared mutable state. Use bounded worker pools and backpressure for fan-out workloads.

**Performance workflow:** Optimize only after measurement (`pprof` and benchmarks). Pre-allocate slices/maps when size is known. Reduce allocations on hot paths (string building, buffer reuse, batching).

**Testing:** Use `httptest` for integration tests. Avoid mocking unless necessary. Use table-driven tests and subtests for business logic. Run race detector for concurrent code paths. Add benchmarks for critical/hot paths.
