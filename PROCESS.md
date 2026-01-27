# PROCESS.md

Goal
- Build a Go implementation of Apprise with a drop-in `apprise` CLI, single-file static builds, and parity tests against the Python package.

Current status
- Scaffolded Go module and a minimal CLI with version output.
- Pinned Go version string to the installed apprise version (1.9.7).
- Added runbook (`AGENTS.md`) and `.gitignore`.
- Added a localhost capture server and Python parity tests (version + JSON payload).
- Implemented a minimal CLI runner, URL parsing, and JSON notifier with Go/Python payload parity tests.
- Added form/xml notifiers with Go/Python parity tests and a schema coverage test driven by the apprise source tree.
- Added request-spec parity capture (monkeypatches Python requests) plus Go request builders for apprise/gotify/ntfy/serverchan/ifttt.
- Standardized request-spec parity into provider folders (`internal/parity/providers/<provider>` with `manifest.json` + `cases.json`) and added provider coverage tests.
- Added SpugPush notifier with parity cases.
- Added SimplePush (spush) notifier with parity cases (unencrypted payloads).
- Added Chanify notifier with parity cases.
- Added Bark notifier with parity cases.
- Added FreeMobile notifier with parity cases.
- Added Google Chat (gchat) notifier with parity cases.
- Added WeCom Bot notifier with parity cases.
- Added Feishu notifier with parity cases.
- Added Lark notifier with parity cases.
- Added Webex Teams notifier with parity cases.
- Added Line notifier with parity cases.
- Added Guilded notifier with parity cases.
- Added PopcornNotify notifier with parity cases.
- Added HttpSMS notifier with parity cases.
- Added D7 Networks (d7sms) notifier with parity cases.
- Added Kavenegar notifier with parity cases.
- Added 46elks notifier with parity cases.
- Added Clickatell notifier with parity cases.
- Added ClickSend notifier with parity cases.
- Relaxed URL parsing to allow empty hosts and numeric-leading schemas (e.g., 46elks) plus Apprise-like path splitting and phone normalization.
- Adjusted request-spec parity to skip JSON comparison on empty bodies.
- Split SMS schemas into a sub-registry (`internal/notify/registry_sms.go`) merged into the main registry.
- Added chat/push sub-registries (`internal/notify/registry_chat.go`, `internal/notify/registry_push.go`) merged into the main registry.
- Added seven notifier with parity cases.
- Added MessageBird (msgbird) notifier with parity cases.
- Added MSG91 notifier with parity cases.
- Added BulkVS notifier with parity cases.
- Added BurstSMS notifier with parity cases.
- Added Sinch notifier with parity cases.
- Added BulkSMS notifier with parity cases.
- Added SMSEagle notifier with parity cases.
- Added SMS Manager notifier with parity cases.
- Added SFR notifier with parity cases.
- Added Vonage (nexmo) notifier with parity cases.
- Added Plivo notifier with parity cases.
- Added Twilio notifier with parity cases.
- Added VoIP.ms notifier with parity cases.
- Added Signal API (signal/signals) notifier with parity cases.
- Added SIGNL4 notifier with parity cases.
- Added WhatsApp notifier with parity cases.
- Added SMTP2Go notifier with parity cases.
- Added SendGrid notifier with parity cases.
- Added SparkPost notifier with parity cases.
- Added Resend notifier with parity cases.
- Added Brevo notifier with parity cases.
- Added Mailgun notifier with parity cases.
- Added Ryver notifier with parity cases.
- Added Zulip notifier with parity cases.
- Added Rocket.Chat notifier with parity cases.
- Added Slack webhook notifier with parity cases.
- Added MSTeams notifier with parity cases.
- Added Revolt notifier with parity cases.
- Added Mattermost notifier with parity cases.
- Added DingTalk notifier with parity cases.
- Added Join notifier with parity cases.
- Added PagerTree notifier with parity cases.
- Added Home Assistant, Synology Chat, Emby, Kumulos, Nextcloud Talk, Streamlabs, Threema, PushSafer, Telegram, LaMetric, BlueSky, Nextcloud (server), and Mastodon notifiers with parity cases.
- Updated request-capture stubs for Emby multi-request flows (login + sessions).

Environment setup
1) Python apprise for parity checks:
   - `cd apprise-go`
   - `python3 -m venv .venv` (recreate if the repo path changes; venv shebangs are path-bound)
   - `.venv/bin/python -m pip install -e ../apprise[all-plugins]` (or `apprise[all-plugins]` if the Python repo is not adjacent)
2) Go tooling:
   - `go version` should be available in PATH

CLI parity basics
- `apprise --version` should match the Python output:
  - `Apprise v<version>`
  - `Copyright (C) 2025 Chris Caron <lead2gold@gmail.com>`
  - `This code is licensed under the BSD 2-Clause License.`

Planned PR stack
1) Scaffold module + CLI + version plumbing + process docs. (In progress)
2) Add localhost capture server + parity test harness (shells out to Python apprise).
3) Implement URL parsing + `json://` plugin + minimal CLI flags needed by tests.

Next steps
- Keep HTTP parity current as upstream adds providers; revisit non-HTTP schemas after the initial release.
- Expand CLI option handling to match apprise usage.

Missing schemas (last parity run, HTTP coverage)
- None (HTTP schemas fully covered).

Ignored schemas (non-HTTP, excluded from coverage for initial release)
- aprs, dbus, gio, glib, gnome, growl, kde, macosx, mqtt, mqtts, qt, rsyslog, smpp, smpps, syslog, windows

Test notes
- `internal/testutil/scripts/capture_request.py` only keeps `User-Agent` when the plugin explicitly sets it (drops requests defaults).
- Parity test runs fix time/nonce/JWT inputs via env defaults to keep AWS/OAuth/VAPID fixtures deterministic.
- Parity tests compare full request sequences by running Go `Send` implementations and capturing all outgoing HTTP requests; request comparisons include method, URL (including query), headers, and body (JSON/form bodies are normalized). Use `-v` to see per-case progress output.
- Capture stubs return canned responses for multi-request providers (e.g., SendPulse OAuth, Emby login/sessions); parity focuses on outbound request shape, not response payloads.
- Python capture caching uses `.tmp/pycapture` by default; set `APPRISE_CAPTURE_CACHE=0` to disable or `APPRISE_CAPTURE_CACHE_DIR` to override.
- Schema case caching uses `.tmp/pycases` by default; set `APPRISE_CASES_CACHE=0` to disable or `APPRISE_CASES_CACHE_DIR` to override.
- Golden fixtures (`internal/parity/providers/<provider>/golden.json`) enable Python-free parity checks; regenerate with `internal/testutil/scripts/update_golden.py`.
- Golden refresh example: `.venv/bin/python internal/testutil/scripts/update_golden.py`.
- CI parity setup uses `scripts/ci/setup_parity_env.sh` and `scripts/ci/run_parity_tests.sh`.
- Always run `go` commands with `GOCACHE=$PWD/.gocache` in this repo to avoid sandboxed cache permission errors.
- Running `go test` in sandboxed environments may require `GOCACHE` set to a writable path and capture-server tests may need local listen permissions.
- Parity subtests run in parallel by default; set `APPRISE_PARITY_SERIAL=1` to force serial execution.

Notes
- Version updates: edit `internal/version/version.go` or override at build time with:
  `-ldflags "-X github.com/unraid/apprise-go/internal/version.Version=<version>"`
