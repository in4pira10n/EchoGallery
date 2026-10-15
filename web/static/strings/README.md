# UI Copy And Locales

## Locale Loading

- `i18n.js` loads the locale saved in the current browser under `echogallery_locale_v1`; otherwise it chooses the best available match for the browser language.
- `locales.json` is the list of languages that can be selected in Settings. Add a catalog such as `en-US.json` and its native-language label here to publish another choice.
- The selected catalog is merged over `zh-CN.json`, so missing translations fall back to Simplified Chinese. Keep interpolation names such as `{count}` stable across catalogs.
- The locale preference is per browser, not per Gallery or account.
- Every new user-visible string must be added to `zh-CN.json` first and then translated in each published catalog. Do not put new UI copy directly in HTML, JavaScript, CSS-generated content, or API response text without a catalog key.
- Keep locale labels and catalog filenames in sync in `locales.json`; the loader must not contain language-specific fallback labels.

## Copy Scope

- Put text visible in App pages, auth/setup/share pages, dialogs, toasts, accessible labels, and backend responses intended for those UI surfaces in locale catalogs.
- Do not add CLI output, server logs, source comments, test descriptions, or internal diagnostic details. Dynamic technical error details may remain as runtime data beside a translated explanation.
- Login and registration `pageTitle` entries are still used for the browser tab title; they are not stale page-body copy.
- Keep backend response keys stable and semantic. Responses can include `error_key` or `message_key` plus the legacy text field; clients resolve the key in the active catalog first.

## Inventory Status

- The App's `app.interface` catalog currently has 644 entries across the main UI, lightbox, and EchoFrame; the copy test checks that every entry has a code reference and every literal reference exists.
- Common App actions have started moving from generated `s_…` / `d_…` identifiers to descriptive keys. Remaining generated keys are still referenced and must not be deleted until the audit reports them unused.
- API responses with stable user-facing text are assigned semantic `error_key` / `message_key` values by the shared response layer and cataloged under `api.errors`, `api.messages`, and `api.status`. The original `error` / `message` fields remain for compatibility. Dynamic errors and task details remain runtime text rather than being mistaken for catalog entries.
- Keep the text audit focused on response payloads consumed by the UI. Go logs, command-line output, comments, and test fixtures are intentionally outside the locale catalog.
- Before publishing, run `node --test tests/i18n-copy.test.cjs`; it verifies catalog structure, interpolation placeholders, page references, and the language manifest.
