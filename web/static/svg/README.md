# EchoGallery SVG Assets

This folder contains inline-loaded SVG icons used by the web UI. Icons are fetched by `web/static/app.js` through the `svgIconFiles` map, then injected into buttons, navigation items, menus, and badges.

## Main Navigation

- `timeline.svg`: Timeline tab icon.
- `favorite.svg`: Favorites tab icon and favorite-related primary UI.
- `album.svg`: Albums tab icon and album placeholders.
- `shuffle.svg`: Random Album tab icon and reshuffle actions.
- `trash.svg`: Trash tab icon and delete-related UI.
- `settings.svg`: Settings tab icon.
- `github.svg`: GitHub repository link in the sidebar.

## Core Actions

- `upload.svg`: Upload actions and upload modal.
- `share.svg`: Share actions in lightbox and dialogs.
- `share-small.svg`: Small share badge shown on shared thumbnails.
- `favorite-small.svg`: Small favorite toggle shown on thumbnails.
- `check.svg`: Selection checkmark on media thumbnails.
- `plus.svg`: Create/add actions.
- `close.svg`: Close buttons.
- `back.svg`: Return/back action that exits the lightbox or returns to the previous UI context.
- `logout.svg`: Log out action.
- `shutdown.svg`: Exit/close the EchoGallery app process.

## Lightbox And Playback

- `prev.svg`: Previous media navigation in lightbox and previous album navigation.
- `next.svg`: Next navigation in lightbox and album navigation.
- `play.svg`: Play button.
- `pause.svg`: Pause button.
- `autoplay.svg`: Autoplay/slideshow-related controls.

## Theme And Layout

- `sun.svg`: Switch to light theme.
- `moon.svg`: Switch to dark theme.
- `pin.svg`: Sidebar pin/auto-hide control.
- `photo.svg`: Generic media placeholder.
- `workshop.svg`: Workshop/customization entry points.

## Context Menu Icons

These icons are used on the left side of right-click context menu items:

- `context-select.svg`: Select or deselect media.
- `context-view.svg`: View/open media in the lightbox.
- `context-timeline.svg`: Jump to the media item in Timeline.
- `context-favorite.svg`: Add to or remove from Favorites.
- `context-reveal.svg`: Reveal media in the system file manager.
- `context-download.svg`: Download media.
- `context-album.svg`: Add to album or open media inside an album.
- `context-share.svg`: Share or manage existing share links.
- `context-delete.svg`: Delete or permanently delete media.
- `context-restore.svg`: Restore media from Trash.

## Conventions

- Keep icons monochrome and use `currentColor` for strokes/fills whenever possible.
- Prefer `viewBox="0 0 24 24"` for UI icons so sizing is consistent.
- Use kebab-case filenames.
- Add new files to the `svgIconFiles` map in `web/static/app.js` when they need to be loaded inline.
