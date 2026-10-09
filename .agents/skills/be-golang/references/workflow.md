# Workflow to work with golang project

## Pre-task checklist

Run before starting any development work.

1. **Confirm scope** — restate the requirement; understand what needs to be done. If ambiguous, ask.
2. **Identify affected modules** identify all modules that need to be created or modified.
3. **Read existing code** — read all related files; identify patterns, conventions, existing helpers. Do not duplicate.
4. **Map impact** — list files to create/modify; trace callers of any shared method being changed.

## Workflows

### New Feature
1. Pre-task checklist.
2. Generate DB migration (if needed).
3. Define proto/API contract follow Technical Design.
4. Implement: Adapter → Manager → Controller.
5. Write integration tests for current changes.
6. Make sure all integration tests pass.
7. Self-review against this skill.

### Bug Fix
1. Pre-task checklist.
2. Reproduce — understand exact input, state, failure.
3. Write integration test with this bug which will fail when trigger with current code.
4. Identify root-cause — do not patch symptoms.
5. Fix minimally — no refactoring in bug-fix scope.
6. Check similar patterns in code for same bug. If found, fix them too.
7. Verify no callers break.
8. Make sure all integration tests pass, include the new test.
9. Self-review against this skill.

### Modify Existing Code
1. Pre-task checklist.
2. Read related implementation and tests, understand current behavior.
3. List all affected features and logic.
4. Modify code and add integration tests for current changes.
5. Update all call sites if signatures change.
6. Make sure all integration tests pass.
7. Self-review against this skill.

### Refactoring
1. Pre-task checklist.
2. Read related implementation and tests, understand current behavior.
3. Identify the pattern that we want to use.
4. List all affected modules.
5. Ensure functionally equivalent — no behaviour change.
6. Apply pattern to refactor, reduce complexity, improve readability, maintainability, and performance. **DO NOT CHANGE ANYTHING IN INTEGRATION TESTS**.
7. Make sure all integration tests pass.
8. Self-review against this skill.

### Improve Performance
1. Pre-task checklist.
2. Read related implementation and tests, understand current behavior.
3. Identify the performance bottleneck.
4. List all affected modules.
5. Apply optimization, reduce complexity, improve readability, maintainability, and performance. **DO NOT CHANGE ANYTHING IN INTEGRATION TESTS**.
6. Make sure all integration tests pass.
7. Self-review against this skill.
