# Explicit production checks

These tests are excluded from the regular browser configuration. They do not
start a server. Do not run them until deployment and test writes are authorized.

Required environment: `E2E_BASE_URL`, `E2E_ALLOW_WRITES=1`, `E2E_RUN_TAG`,
`E2E_COMPANY_EMAIL`, `E2E_COMPANY_PASSWORD`, `E2E_DEVELOPER_EMAIL`, and
`E2E_DEVELOPER_PASSWORD`. Use two isolated accounts whose email local parts
contain the exact run tag (lowercase 8–64 characters including `e2e`). Passwords
must contain at least 12 characters. The company owner must own exactly one
company whose name contains the run tag, or set `E2E_COMPANY_ID` to select one
owned tagged company when testing a multi-company account. Supply secrets through the process
environment, never command arguments or committed files.

Run from `frontend`: `npx playwright test --config=playwright.production.config.ts`.
Default mode uses confirmed accounts: UI login, draft creation, publication,
application, selection, and the developer's accepted status. Canonical read API
calls independently verify persistent states. Browser contexts separate actors.

For the optional public signup stage, additionally set `E2E_REGISTER_UI=1`,
`E2E_EMAIL_POLICY=confirmation`, and `E2E_ALLOW_EMAIL=1`. This sends signup mail
only to the two operator-supplied addresses, verifies the confirmation screen,
and stops with `awaiting_email_confirmation`. It does not prove email delivery.
Confirm through the actual inbox before running the default lifecycle mode.
Do not reuse accounts created by the API registration runner for signup again.

No screenshots, videos, traces, or raw error logs are saved. Do not override the
safe reporter or enable debugging/tracing: these can expose form credentials.
Private manifests default to `/tmp/firstorder-browser-*.json` and include only
test resource IDs, safe step names, and status. Set `E2E_BROWSER_MANIFEST_FILE`
to choose a new, nonexistent file. They remain for reviewed cleanup even after
failure. No automatic deletion or change of Auth confirmation policy occurs.

The API runner `backend/tools/e2e_registration_to_selection.py` supports a
separately approved admin-generated confirmation-token stage. That is explicitly
reported as bypassing email delivery and **does not validate public signup or
SMTP**. The browser lifecycle remains useful after such account preparation.

Safe discovery only (no browser or HTTP requests): with origin/write gates set,
append `--list`. Do not treat discovery as a live test result.
