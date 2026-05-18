# CodeRabbit Fixes WIP

## Context

- Repo: unraid/apprise-go
- Branch: codex/fix-format-input-handling
- PR: #50
- PR URL: https://github.com/unraid/apprise-go/pull/50
- Generated at: 2026-05-18T16:22:30Z

## Inputs Pulled

- [x] Unresolved CodeRabbit review threads pulled
- [x] Top-level CodeRabbit review notes pulled
- [x] Top-level actionable review-body comments extracted into queue

## Fix Queue

| Item ID | Type | File | Line | Summary | Status | Link | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CR-001 | thread | internal/notify/format_convert.go | 102 | HTML to Markdown currently uses HTML to text. Verified current code does this, but upstream Python Apprise has the same contract in conversion.py. | BLOCKED | https://github.com/unraid/apprise-go/pull/50#discussion_r3260222516 | Skipped to preserve Python parity. |
| CR-002 | thread | internal/cli/cli_test.go | 154 | Convert added CLI Run Telegram/mailto tests from Go-only assertions to Python-vs-Go parity. | DONE | https://github.com/unraid/apprise-go/pull/50#discussion_r3260277570 | `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil` passed. |
| CR-003 | thread | internal/notify/mailto_format_test.go | 31 | Use `net.JoinHostPort` when building mailto test authority. | DONE | https://github.com/unraid/apprise-go/pull/50#discussion_r3260357765 | `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil` passed. |
| CR-004 | thread | internal/testutil/request_compare.go | 50 | Use `strings.EqualFold` for method comparison. | DONE | https://github.com/unraid/apprise-go/pull/50#discussion_r3260357786 | `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil` passed. |
| RVW-001 | review-body | top-level | n/a | Earlier review body with six inline comments. | DONE | https://github.com/unraid/apprise-go/pull/50#pullrequestreview-4311662350 | Covered by prior commits plus CR-001 skip reason. |
| RVW-002 | review-body | top-level | n/a | Review body requested Python parity for CLI Run Telegram/mailto format tests. | DONE | https://github.com/unraid/apprise-go/pull/50#pullrequestreview-4311723839 | Covered by CR-002. |
| RVW-003 | review-body | internal/notify/telegram_format_test.go | 34-55 | Add Python parity to MarkdownV2 title escaping test. | BLOCKED | https://github.com/unraid/apprise-go/pull/50#pullrequestreview-4311819314 | Skipped: Python Apprise MarkdownV2 title/body blending differs from the Go-side hardening test, so adding this assertion would fail for an implementation-contract mismatch rather than guarding the reserved-character escaper. |
| RVW-004 | review-body | internal/notify/mailto_format_test.go | 31 | Latest review-body mailto IPv6-safe authority item. | DONE | https://github.com/unraid/apprise-go/pull/50#pullrequestreview-4311819314 | Covered by CR-003. |
| RVW-005 | review-body | internal/testutil/request_compare.go | 49-50 | Latest review-body `strings.EqualFold` item. | DONE | https://github.com/unraid/apprise-go/pull/50#pullrequestreview-4311819314 | Covered by CR-004. |

## Execution Log

### 1. Item: CR-002
- Action: Replaced Go-only CLI Telegram HTTP assertions with `assertRunHTTPRequestParity`, using Python request capture and Go `Run(...)` under `CaptureGoRequests`. Replaced the mailto CLI test with Python CLI send plus Go `Run(...)` against the shared SMTP capture, comparing normalized SMTP output.
- Validation: `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil`
- Result: DONE

### 2. Item: CR-003
- Action: Changed mailto format test URL construction to use `net.JoinHostPort(host, port)`.
- Validation: `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil`
- Result: DONE

### 3. Item: CR-004
- Action: Replaced `strings.ToUpper(...) != strings.ToUpper(...)` with `!strings.EqualFold(...)`.
- Validation: `go test -count=1 ./internal/cli ./internal/notify ./internal/testutil`
- Result: DONE

### 4. Item: CR-001
- Action: Verified against `../apprise/apprise/apprise/conversion.py`; Python Apprise currently maps HTML to Markdown through `html_to_text`.
- Validation: Code inspection.
- Result: BLOCKED; skipped to preserve parity.

### 5. Item: RVW-003
- Action: Verified the request would compare Go MarkdownV2 title escaping against Python's different Markdown title/body blending behavior.
- Validation: Code inspection of `../apprise/apprise/apprise/plugins/base.py` and `../apprise/apprise/apprise/plugins/telegram.py`.
- Result: BLOCKED; skipped as not a valid parity assertion against current upstream behavior.

## Final Checks

- [x] Queue reviewed: no `TODO` left
- [x] Remaining `BLOCKED` items documented with reason
- [x] Re-pulled CodeRabbit threads and reviews
- [x] No unhandled top-level review-body comment remains

Re-pull result: one unresolved CodeRabbit inline thread remains (`CR-001`) and is intentionally `BLOCKED` because the requested HTML to Markdown change conflicts with current upstream Python Apprise conversion behavior. The remaining top-level review-body entries are duplicates or aggregates mapped to queue items above.
