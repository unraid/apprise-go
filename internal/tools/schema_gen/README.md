# schema_gen

Emits a Go schema entry literal from upstream's own plugin details.

    python3 internal/tools/schema_gen/main.py <schema> [order] >> internal/notify/<provider>.go

Schema entries are pure data derived from upstream, so transcribing them by
hand is tedious and drifts. Generating them makes `TestSchemaMetadataParity`
pass by construction, which leaves the actual work — the provider behaviour —
as the thing to get right.

Requires `.venv` built from the pinned upstream (`scripts/ci/setup_parity_env.sh`).

Fill in by hand after generating, since these are apprise-go's own
classification rather than upstream's: `attachment_support`, `category`,
`service_name`, `service_url`, `setup_url`, and the registration order.

**A generated schema entry is not support.** Registering a schema whose
provider is not implemented turns `TestSchemaCoverage` green while the URL
does nothing, which is worse than the gap it appears to close. Generate the
entry as part of porting the provider, never ahead of it.
