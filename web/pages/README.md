# EchoGallery Pages

`web/pages` contains page-level HTML and CSS entry points. Keep page shells here instead of embedding large HTML strings in Go files.

## Route Map

| Route | HTML | CSS | Notes |
| --- | --- | --- | --- |
| `/`, `/albums`, `/albums/:id`, `/trash` | `app.html` | `app.css` | Main EchoGallery SPA shell. The app logic still lives in `web/static/app.js`. |
| `/login` | `login.html` | `common.css`, `login.css` | Login page. Edit these files for login layout and visual changes. |
| `/register` | `register.html` | `common.css`, `login.css` | User registration page. It shares the standalone auth-page visual language with login. |
| `/gallery-chooser` | `gallery-chooser.html` | `common.css`, `setup.css`, `gallery-chooser.css` | PWA fallback page shown when the current Gallery server cannot be reached. |
| setup server `/` | `setup.html` | `common.css`, `setup.css` | First-run setup and library recovery page. The placeholder `__SETUP_PAYLOAD__` is replaced by Go at runtime. |

## Compatibility Notes

- `web/static/app.js` remains the main application script because the current SPA shares state and rendering across tabs.
- `web/static/app.css` is kept as a legacy static stylesheet. New page-level styling should go in `web/pages/*.css`.
- Shared standalone-page copy can be centralized in `web/static/strings/zh-CN.json`. Login, register, setup, and part of the SPA load text from there with built-in fallbacks.
- The Go server prefers files in `web/pages`, but falls back to embedded defaults if a page file is missing or invalid. This prevents startup failure or blank pages during packaging mistakes.
- When adding a new page file, include it in `embed.go` by keeping it under `web/pages`, then add or update a route in `internal/api`.

## Safe Editing Guide

- Main app visual changes: edit `app.css`, and only touch `web/static/app.js` when behavior changes.
- Login or registration page changes: edit `login.html`, `register.html`, and `login.css`.
- Offline Gallery chooser changes: edit `gallery-chooser.html`, `gallery-chooser.css`, and `web/static/gallery-chooser.js`.
- Setup or resource-library recovery changes: edit `setup.html` and `setup.css`.
- Shared standalone-page controls such as `.btn`, `.input`, `.form-group`: edit `common.css`.
- Batch copy changes: edit `web/static/strings/zh-CN.json`.
