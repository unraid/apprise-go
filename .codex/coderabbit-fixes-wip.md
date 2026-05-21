# CodeRabbit Fixes WIP

## Context

- Repo: unraid/apprise-go
- Branch: codex/telegram-formatting-followup
- PR: #60
- PR URL: https://github.com/unraid/apprise-go/pull/60
- Generated at: 2026-05-21T00:36:00-04:00

## Inputs Pulled

- [x] Unresolved CodeRabbit review threads pulled
- [x] Top-level CodeRabbit review notes pulled
- [x] Top-level actionable review-body comments extracted into queue

## Fix Queue

| Item ID | Type | File | Line | Summary | Status | Link | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CR-001 | thread | internal/notify/format_convert_test.go | 168 | Add Python parity to the cross-target corpus test. | BLOCKED | https://github.com/unraid/apprise-go/pull/60#discussion_r3278534673 | Skipped: this corpus intentionally validates target format conversion behavior that fixes Telegram behavior beyond current Python Apprise. Adding Python request parity would lock in the upstream bug this PR is fixing. |
| CR-002 | thread | internal/notify/telegram_format_test.go | 140 | Route new Telegram format tests through Python-vs-Go request-sequence parity. | BLOCKED | https://github.com/unraid/apprise-go/pull/60#discussion_r3278534678 | Skipped: these tests assert corrected Telegram parse payloads that current Python Apprise does not emit. |
| CR-003 | thread | internal/notify/live/telegram_live_test.go | 101 | Add Python-apprise request parity to the live Telegram test. | BLOCKED | https://github.com/unraid/apprise-go/pull/60#discussion_r3278534680 | Skipped: live test purpose is Bot API acceptance of corrected Go-generated Telegram parse payloads, not matching upstream Python's currently broken formatting. |
| CR-004 | thread | internal/notify/live/telegram_live_test.go | n/a | Require explicit destination for the live suite. | DONE | https://github.com/unraid/apprise-go/pull/60#discussion_r3278534682 | Addressed in `a3dd48f`; GraphQL reports thread resolved/outdated. |
| RVW-001 | review-body | internal/notify/live/telegram_live_test.go | 140-149 | Redact bot token from live test transport error logs. | DONE | https://github.com/unraid/apprise-go/pull/60#pullrequestreview-4320292462 | Added redaction helper and regression test; targeted and full Go tests passed. |

## Execution Log

### 1. Item: CR-004
- Action: Required `APPRISE_GO_TELEGRAM_CHAT_ID` for live validation and removed auto-discovery/bot-ID fallback.
- Validation: `go test ./internal/notify/live -run TestTelegramLiveFormattingAgainstBotAPI -count=1 -v` with explicit live env passed.
- Result: DONE

### 2. Item: CR-001
- Action: Verified this asks for parity against Python Apprise behavior that does not include the corrected Telegram conversions under test.
- Validation: Code/test review.
- Result: BLOCKED; skipped because it conflicts with the bug fix goal.

### 3. Item: CR-002
- Action: Verified the requested parity would force the new Telegram unit tests back to current Python output.
- Validation: Code/test review.
- Result: BLOCKED; skipped because it conflicts with the bug fix goal.

### 4. Item: CR-003
- Action: Verified live validation is intentionally testing Bot API acceptance of Go-generated corrected payloads.
- Validation: Code/test review.
- Result: BLOCKED; skipped because Python parity is not the purpose of this live suite.

### 5. Item: RVW-001
- Action: Added token redaction before reporting request creation or transport errors.
- Validation: `go test ./internal/notify/live -count=1`; `go test ./internal/notify ./internal/notify/live -run 'TestTelegram|TestTargetFormatConversionCorpusAcrossWorkflowTargets' -count=1`; live Bot API test with explicit env; `go test ./...`
- Result: DONE

## Final Checks

- [x] Queue reviewed: no `TODO` left
- [x] Remaining `BLOCKED` items documented with reason
- [ ] Re-pulled CodeRabbit threads and reviews
- [ ] No unhandled top-level review-body comment remains
