# Long integer table sorting evidence

Code baseline: multica-ai/multica `282f48eed2531f467ed4e6febc2a59b590d3e711`.
Fix: jiahao6635/multica `3a47cfc76d85cb7f14804a464aaae4c48cf6c5ac`.

These are screenshots of the running Next.js web app, opening a Markdown table from a chat message into its normal expanded dialog. HTTP API responses and WebSockets are synthetic Playwright fixtures; no real account, credential, agent, or paid task was used. The source strings remain unchanged. Before ascending is incorrect; after ascending and descending are correct. The chosen input already happens to be descending, so the before-descending image also has correct order.

`table-sort-evidence.spec.ts` is the exact final diagnostic fixture. Copy it into the checkout's `e2e/` directory. Start the web app, ensure Chromium is at `/usr/bin/chromium`, create `/workspace/table-sort-evidence`, then run:

```sh
SORT_STAGE=after PLAYWRIGHT_BASE_URL=http://localhost:13971 pnpm exec playwright test e2e/table-sort-evidence.spec.ts --reporter=line --timeout=120000
```

Use `SORT_STAGE=before` against the baseline to assert the old ascending result. The final fixture checks page errors, API schema warnings and unexpected API paths; the after run passed. The original before screenshot fixture used fallback-compatible empty arrays for two unrelated APIs; the final fixture uses their object response schemas. Early browser attempts timed out during cold compilation/readiness and one concurrent validation run timed out waiting for the table; the final isolated run passed in 13.3 seconds.

The managed `make up C=api,web` first timed out during cold web compilation, and a subsequent invocation could not resolve the API listener identity using lsof. The screenshot run used the existing frontend dev command on the same local frontend port. API requests were fully intercepted, so it does not establish real backend login, chat persistence or attachment upload/opening behavior.

Not verified: real CSV/TSV uploads in a browser, a real authenticated backend flow, Electron/native platforms, other browsers, production builds, performance or real agent execution. CSV/TSV sort wiring and empty-last behavior are covered by React/jsdom tests with virtualization stubbed. All 1,898 editor/rich-content tests, views typecheck and changed-file lint passed on the fix.

AI assistance: OpenAI Codex investigated, implemented, tested and prepared these artifacts.
