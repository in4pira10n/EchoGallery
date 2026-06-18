# EchoGallery SVG Assets

This folder contains inline-loaded SVG icons used by the web UI. Icons are fetched by `web/static/app.js` through the `svgIconFiles` map, then injected into buttons, navigation items, menus, and badges.

## Main Navigation

- `timeline.svg`: Timeline tab icon.
- `favorite.svg`: Favorites tab icon and favorite-related primary UI.
- `memories.svg`: Memories tab icon.
- `album.svg`: Albums tab icon and album placeholders.
- `shuffle.svg`: Random Album tab icon and reshuffle actions.
- `trash.svg`: Trash tab icon and delete-related UI.
- `settings.svg`: Settings tab icon.
- `github.svg`: GitHub repository link in the sidebar.

## Core Actions

- `upload.svg`: Upload actions and upload modal.
- `download.svg`: Download actions in icon-only top bars and lightbox controls.
- `share.svg`: Share actions in lightbox and dialogs.
- `share-small.svg`: Small share badge shown on shared thumbnails.
- `favorite-small.svg`: Small favorite toggle shown on thumbnails.
- `favorite-filled.svg`: Filled favorite state for media already added to Favorites.
- `check.svg`: Selection checkmark on media thumbnails.
- `plus.svg`: Create/add actions.
- `library-create.svg`: Floating Settings action for creating a new library.
- `advanced-settings.svg`: Settings summary entry for advanced options.
- `topbar-load-all.svg`: Topbar icon-only action for loading all media in a view.
- `topbar-upload.svg`: Topbar icon-only upload action.
- `topbar-download-favorites.svg`: Topbar action for downloading all favorites.
- `topbar-shuffle.svg`: Topbar action for reshuffling random albums.
- `topbar-return-random-position.svg`: Topbar action for returning to the last viewed random-album position.
- `topbar-save-restart.svg`: Topbar save-and-restart action.
- `topbar-save.svg`: Topbar save action.
- `topbar-new-album.svg`: Topbar create-album action.
- `topbar-prev-album.svg`: Topbar previous-album navigation.
- `topbar-next-album.svg`: Topbar next-album navigation.
- `topbar-back-albums.svg`: Topbar return-to-albums action.
- `topbar-download-album.svg`: Topbar download-album action.
- `topbar-delete-album.svg`: Topbar delete-album action.
- `topbar-empty-trash.svg`: Topbar empty-trash action.
- `topbar-clear-memories-action.svg`: Topbar clear-memories action.
- `topbar-restore-selected.svg`: Batch restore action in Trash selection bar.
- `topbar-timeline-order.svg`: Topbar timeline order toggle.
- `album-view-grid.svg`: Album view toggle for grid mode.
- `album-view-list.svg`: Album view toggle for list mode.
- `album-sort-timeline-desc.svg`: Album detail sort control for newest/timeline descending order.
- `album-sort-timeline-asc.svg`: Album detail sort control for oldest/timeline ascending order.
- `album-sort-name.svg`: Album detail sort control for sorting by media name.
- `album-sort-size.svg`: Album detail sort control for sorting by file size.
- `album-card-folder.svg`: Placeholder icon slot for folder-import album cards.
- `album-card-user.svg`: Placeholder icon slot for manually-created album cards.
- `album-card-count.svg`: Placeholder icon slot shown before the item count on album cards.
- `floating-search.svg`: Dedicated oversized search glyph for the bottom-right floating search button.
- `grid-scale-button.svg`: Dedicated floating control for cycling photo-wall thumbnail sizes.
- `close.svg`: Close buttons.
- `back.svg`: Return/back action that exits the lightbox or returns to the previous UI context.
- `more.svg`: More actions button, especially for touch-friendly access to context menus.
- `logout-1.svg`: Log out action in the sidebar.
- `logout.svg`: Power-style logout/exit glyph kept for compatibility with older UI references.
- `shutdown.svg`: Exit/close the EchoGallery app process.

## Lightbox And Playback

- `prev.svg`: Previous media navigation in lightbox and previous album navigation.
- `next.svg`: Next navigation in lightbox and album navigation.
- `play.svg`: Play button.
- `pause.svg`: Pause button.
- `autoplay.svg`: Autoplay/slideshow-related controls.
- `slideshow-loop.svg`: Lightbox slideshow loop toggle.
- `fit.svg`: Fit media to the current viewport in the lightbox.
- `info-type.svg`: Lightbox metadata icon for media type.
- `info-mime.svg`: Lightbox metadata icon for MIME/format.
- `info-date.svg`: Lightbox metadata icon for capture/import time.
- `info-size.svg`: Lightbox metadata icon for file size.
- `info-ratio.svg`: Lightbox metadata icon for dimensions and aspect ratio.
- `info-dimensions.svg`: Lightbox metadata icon for pixel dimensions.
- `info-file-size.svg`: Lightbox metadata icon for file size.
- `exif-camera.svg`: EXIF camera brand/model metadata.
- `exif-aperture.svg`: EXIF aperture metadata.
- `exif-shutter.svg`: EXIF exposure time metadata.
- `exif-iso.svg`: EXIF ISO metadata.
- `exif-focal.svg`: EXIF focal length metadata.
- `exif-gps.svg`: EXIF GPS metadata.
- `exif-orientation.svg`: EXIF orientation metadata.
- `media-all.svg`: Topbar media filter icon for showing all media types.
- `media-image.svg`: Lightbox metadata icon for image media.
- `media-video.svg`: Lightbox metadata icon for video media.
- `timeline-order.svg`: Topbar Timeline sort-order toggle.
- `video-bookmark.svg`: Lightbox video playback bookmark action.

## Theme And Layout

- `sun.svg`: Switch to light theme.
- `moon.svg`: Switch to dark theme.
- `pin.svg`: Sidebar pin/auto-hide control.
- `menu.svg`: Sidebar compact/expanded layout toggle.
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
- `context-convert-playback.svg`: Convert an unsupported video into a browser-playable MP4 cache from the context menu.

## Conventions

- Keep icons monochrome and use `currentColor` for strokes/fills whenever possible.
- Prefer `viewBox="0 0 24 24"` for UI icons so sizing is consistent.
- Use kebab-case filenames.
- Add new files to the `svgIconFiles` map in `web/static/app.js` when they need to be loaded inline.
