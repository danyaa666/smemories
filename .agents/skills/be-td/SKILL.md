---
name: be-td
description: >
  Technical Design Document (TDD) generation from PRDs. Use to drive end-to-end design:
  parse requirements, gather codebase context, resolve ambiguities, draft complete TDD.
  Ensure all output is strictly relevant to the technical document itself.
---

# 1. Pre-Design Checklist

Complete before writing any design.

### Step 1 — Gather Sources
- Read the PRD on Confluence (use MCP if available).
- Read all linked Jira tickets + subtasks (scope, acceptance criteria, constraints).
- Review attached UI mockups before reading existing code.

### Step 2 — Summarize Requirements
- Write 3–5 sentence plain-language summary: what the feature does and why.
- List actors and systems involved.
- Identify core business outcomes.
- If summary reveals ambiguity → stop and ask.

### Step 3 — Explore Codebase
- Identify touched services, modules, tables.
- Read relevant Controller/Manager/Repository/proto/DB code.
- Note reusable patterns, helpers, shared components.
- Distinguish net-new vs modifying existing behaviour.

### Step 4 — Surface Open Questions
Group into one consolidated list (ask all at once, not across turns):

- **Functional:** Edge cases, ambiguous business rules, sequencing.
- **Non-functional:** Volume (RPS, records/day), latency, retention, consistency model.
- **Scope:** What's explicitly in/out, dependent services.

---

# 2. When to Start Writing

Begin only after:
1. All sources read.
2. Summary confirmed by engineer/PM.
3. All questions answered or deferred as documented assumptions.

---

# 3. TDD Structure

Include all applicable sections. Omit only with explicit justification.

## Section 1 — Overview

```markdown
## Overview
**Feature:** <Name>  |  **PRD:** <link>  |  **Epic:** <link>  |  **Tickets:** <links>
**Author:** <name>  |  **Date:** YYYY-MM-DD  |  **Status:** Draft | In Review | Approved

<One paragraph technical summary>
```

## Section 2 — Functional Requirements

Numbered, testable "SHALL" statements. No implementation details.

```
1. The system SHALL allow staff to create a customer account using email and phone.
2. The system SHALL reject creation if email is already registered.
```

## Section 3 — Non-Functional Requirements

| Concern | Requirement |
|---|---|
| Latency | p95 target |
| Throughput | Peak RPS |
| Availability | Uptime |
| Consistency | Strong / eventual |
| Scalability | 12-month growth |
| Security | Auth, validation, sensitive data |
| Observability | Metrics, logs, alerts |
| Retention | Duration, archival policy |

## Section 4 — Architecture Overview → `@be-architect`

**Include when:** new service/module, changed service interactions, async processing, cross-service deps.
**Omit when:** self-contained single-service change (state why).

Contents: Mermaid component diagram + component descriptions + communication patterns.

## Section 5 — Database & Search Design

**Use these skills as standards if necessary, DO NOT print out this subsection in the generated TDD**: `@be-rldb` (relational), `@be-clickhouse` (analytics), `@be-es` (search).

For each new/modified table or index:

1. **Purpose** — one sentence.
2. **Schema** — full DDL or mapping JSON.
3. **Column/Field Notes** — non-obvious columns.
4. **Index Rationale** — each index + query pattern it supports.
5. **Capacity Estimation:**

| Metric | Value | Calculation |
|---|---|---|
| Daily writes | X rows/day | e.g., 10k DAU × 2 actions |
| Avg row size | X bytes | Column sum + 20% overhead |
| Index overhead | X% | 30–100% typical |
| Growth rate | X% MoM | |
| 1-year total | X GB | Adjusted for growth + indexes |
| Read:Write ratio | X:1 | |

6. **Lifecycle:** Compression strategy, TTL/truncation plan, archival (cold storage).
7. **Migration:** Filename per convention.

### AI INSTRUCTION: Storage Review Trigger
**CRITICAL:** Do NOT print this subsection in the generated TDD. 
If your capacity estimation determines any of the following conditions are met, you MUST pause the generation process and ask the human user for review/consideration before finalizing the storage design:
- 1-year storage > 100GB for a single table
- Index overhead > 100% of raw data
- High-frequency append-only without a truncation/archival plan
- Daily volume suggests an IOPS bottleneck

## Section 6 — Business Logic Flows

Every significant flow requires:
1. **Mermaid sequence diagram** — include client as leftmost participant, show DB as named participant, show all error paths with `alt`/`else`, use `Note over` for state changes.
2. **Numbered prose walkthrough.**
3. **Client Integration Notes** — non-obvious behaviours, retry guidance, error handling.

## Section 7 — API Design → `@be-api-design`

Group by service/namespace. Every endpoint must include:
1. Method + path (HTTP) or RPC name (gRPC)
2. Auth requirement
3. Request field table (name, type, required, constraints, description)
4. Response field table (name, type, description)
5. Business error codes with descriptions
6. Example request + response

Define shared response shapes as **Common Objects** at section top.

## Section 8 — Cross-Cutting Concerns

Address each (N/A if not applicable):
- **Auth:** Public vs authenticated endpoints, role-based access, identity validation.
- **Idempotency:** Which writes, how enforced, duplicate handling.
- **Concurrency:** Shared resources, locking strategy, TOCTOU risks.
- **Error/Retries:** Retryable vs permanent errors, partial failure handling.
- **Observability:** Key metrics, log events + levels, production alerts.
- **Privacy/Security:** PII handling, fields never logged, input validation.

## Section 9 — Test Plan

### 9.1 Unit Tests

Cover the majority of business logic in isolation. Mock all external dependencies (DB, cache, external services).

**Scope:** One test file per Manager/Service/Repository/Helper. Aim for ≥ 80% coverage on business logic layers.

For each component, cover:

| Category | Cases to Cover |
|---|---|
| Happy path | Valid input → expected output/state change |
| Input validation | Missing required fields, wrong types, out-of-range values, empty strings |
| Business rule violations | Duplicate create, insufficient balance, invalid state transition, expired token, etc. |
| Edge cases | Zero values, max values, boundary conditions, empty collections |
| Error propagation | Dependency returns error → correct error type wrapped and surfaced |
| Idempotency | Repeated calls with same inputs produce same result without side effects |
| Permission / auth logic | Correct role passes, wrong role rejected, missing identity rejected |

**Format per test:**
```go
func TestXxx_Scenario_ExpectedBehaviour(t *testing.T) {
    // Arrange
    // Act
    // Assert
}
```

**Naming convention:** `Test<Component>_<Scenario>_<ExpectedResult>`

---

### 9.2 Integration Tests

Cover end-to-end flows through real DB / real dependencies (no mocks for infra). Focus on **happy paths** and critical failure paths that unit tests cannot catch.

**Scope:** One test suite per feature or API group. Run against a test DB with seed data.

Mandatory happy-path cases:

| Flow | What to Verify |
|---|---|
| Create resource | Record persisted, response matches, side effects triggered (events, notifications) |
| Read / list resource | Correct data returned, pagination works, filters applied |
| Update resource | Fields updated, version/timestamp bumped, audit trail recorded |
| Delete / deactivate | Record marked inactive or removed, cascades correct |
| Auth-protected endpoints | Valid token → 200, missing token → 401, wrong role → 403 |
| Cross-service flows | Downstream service called, data consistent across services |

Include at least one negative integration case per flow:
- Duplicate create → correct conflict error returned
- Invalid FK / missing parent → correct error returned

**Test data strategy:**
- Seed minimal data per test (avoid shared state).
- Use transactions rolled back per test where possible.
- Document any fixed seed data requirements.

---

### 9.3 Performance Tests

**Include when any of the following apply:**
- Endpoint expected > 100 RPS sustained
- DB query touches table > 1M rows
- Flow involves fan-out (batch writes, bulk notifications, parallel external calls)
- SLA requires p95 < 200ms or similar latency target (from NFRs in Section 3)

**Otherwise:** state "Performance testing not required for this feature" with rationale.

#### Scenarios to cover (when applicable):

| Scenario | Target | Tool |
|---|---|---|
| Baseline throughput | Sustain peak RPS from NFRs with p95 ≤ SLA | k6 / Locust |
| Spike / burst | 5× normal load for 30s, recover within 60s | k6 |
| Large dataset queries | Query against table at projected 12-month size | Direct DB bench |
| Concurrent writes | N concurrent requests on same resource, no deadlock / data corruption | k6 |
| Cache effectiveness | Hit ratio ≥ target under load | k6 + metrics |

**Acceptance criteria:** All scenarios pass SLA targets defined in Section 3 NFRs. Document results in TDD after load test run.

---

### 9.4 Test Plan Summary Table

| Layer | Coverage Target | Owner | Environment |
|---|---|---|---|
| Unit | ≥ 80% business logic | Feature dev | Local / CI |
| Integration | All happy paths + critical failures | Feature dev | Staging DB |
| Performance | Per scenarios above (if applicable) | Feature dev + SRE | Perf environment |

---

# 4. Review Checklist

**Completeness:**
- [ ] All 9 sections present or omitted with justification
- [ ] Open questions answered or documented as assumptions
- [ ] PRD + Jira links included

**Requirements:**
- [ ] Functional: testable SHALL statements covering all user journeys
- [ ] Non-functional: all concerns addressed

**Database:** → `@be-rldb` standards
- [ ] Capacity + growth estimated for all new tables/indexes
- [ ] Lifecycle strategy defined
- [ ] Human review flagged if thresholds exceeded
- [ ] Migration filenames specified

**Flows:**
- [ ] Sequence diagram per significant journey
- [ ] Error paths in all diagrams
- [ ] Client integration notes per flow

**API:** → `@be-api-design` standards
- [ ] Every endpoint fully documented with examples
- [ ] All business error codes listed

**Cross-cutting:**
- [ ] Idempotency for all writes
- [ ] Concurrency risks mitigated
- [ ] Observability specified

**Test Plan:**
- [ ] Unit tests cover happy path, validation, business rules, edge cases, error propagation
- [ ] Integration tests cover all happy paths and critical failure paths
- [ ] Performance tests included if RPS/latency SLAs or large datasets apply
- [ ] Test coverage target stated and owner assigned per layer

---

# 5. Output Format

- Markdown with ATX headings (`#`, `##`, `###`).
- Fenced code blocks with language tags (sql, json, protobuf, mermaid).
- Properly formatted Markdown tables.
- Self-contained — reader should not need follow-up questions to implement.