# CodeRabbit Fixes WIP

## Context

- Repo: unraid/apprise-go
- Branch: codex/cli-help-parity
- PR: #62
- PR URL: https://github.com/unraid/apprise-go/pull/62
- Generated at: 2026-05-21T14:29:52Z

## Inputs Pulled

- [x] Unresolved CodeRabbit review threads pulled
- [x] Top-level CodeRabbit review notes pulled
- [x] Top-level actionable review-body comments extracted into queue

## Fix Queue

| Item ID | Type | File | Line | Summary | Status | Link | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CR-001 | thread | internal/cli/cli.go | 212 | Unknown-option detection is over-applied and can misclassify non-unknown parse errors. | DONE | https://github.com/unraid/apprise-go/pull/62#discussion_r3281866028 | `go test ./internal/cli` passed; exact `-R not-an-int --blah` behavior is now compared against Python Apprise. |
| RVW-001 | review-body | top-level | n/a | Review body reports one actionable comment, represented by CR-001. | DONE | https://github.com/unraid/apprise-go/pull/62#pullrequestreview-4337724219 | No separate top-level actionable item beyond CR-001. |

## Execution Log

### 1. Item: CR-001
- Action: Verified the exact installed Python Apprise behavior for `-R not-an-int --blah` and updated the regression test to compare Go against Python instead of asserting a hand-written expected result.
- Validation: `go test ./internal/cli` passed.
- Result: DONE

## Final Checks

- [x] Queue reviewed: no `TODO` left
- [x] Remaining `BLOCKED` items documented with reason
- [x] Re-pulled CodeRabbit threads and reviews
- [x] No unhandled top-level review-body comment remains
