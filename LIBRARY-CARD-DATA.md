# Library Card Data

This note documents where resource-library card data lives. Keep this split intact so switching views or re-rendering settings does not make cards lose availability, stats, or preview data.

## Persistent config

Stored in `config.json` as `libraries`.

- `id`
- `name`
- `path`
- `logo_asset`
- `accent_color`
- `status`
- `owner_username`

Only these fields should be sent back when saving settings.

## User profile state

Stored in each user's `profile.json`.

- `active_library_id`
- `storage_path`
- personal preferences

The profile points to the selected global library. It should not store the full library list.

## Runtime card data

Returned by `GET /api/settings` and kept in `state.serverSettings.libraries`.

- `available`
- `unavailable_reason`
- `locked_by_username`
- `logo_image_url`
- `created_at`
- `last_scanned_at`
- `unsupported_media_count`
- `total_size_text`
- `photo_count`
- `video_count`

These fields are display-only. The settings UI must preserve them while re-rendering or switching tabs, but must not persist them into `config.json`.

## Frontend rule

When collecting card drafts from DOM, merge the edited fields with the current runtime library object by `id`, falling back to `path`. This prevents cards from becoming unavailable or losing stats after switching to another tab and returning.
