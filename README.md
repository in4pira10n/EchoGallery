# EchoGallery

[English](README.md) | [简体中文](docs/README.zh-CN.md)

The next-generation local photo gallery management app.

<img src="https://github.com/user-attachments/assets/dae16cf6-55c7-4c46-be5c-136dfb5ebd53" />

## Introduction

EchoGallery is a local web-based photo gallery for browsing, managing, and sharing photos and videos on your computer or local network.

It is designed for people who want:

- A lightweight, locally hosted media library
- Privacy without depending on cloud services
- Timeline, albums, favorites, and trash management for local media
- Simple share links for sharing selected photos with family or friends

## Features

- Timeline browsing
- Full-size media viewing with a lightbox
- Random album
- Ordered and random slideshow playback
- Link-based sharing (beta)
- Library management
- Portable libraries (`.echogallery` travels with the library folder)
- Smart albums
- Favorites and trash
- Video thumbnail refresh

> Video poster generation depends on `ffmpeg`. Videos can still be managed without `ffmpeg`, but their thumbnails may not be generated.

## Keyboard Shortcuts

### Global shortcuts

- `Alt + 1`: Open Timeline
- `Alt + 2`: Open Favorites
- `Alt + 3`: Open Random Album
- `Alt + 4`: Open Albums
- `Alt + 5`: Open Memories
- `Alt + 6`: Open Trash
- `Alt + 7`: Open Settings

### Full-size media viewer

- `Esc`: Close the viewer
- `A`: Previous media
- `D`: Next media
- `Space`: Toggle slideshow playback
- `F`: Add or remove from Favorites
- `Alt + 0`: Return to Fit mode

### Video player shortcuts (system player)

- `X`: Pause / play
- `Z`: Seek backward 5 seconds
- `C`: Seek forward 5 seconds
- `W`: Increase volume
- `S`: Decrease volume
- `M`: Mute
- `F`: Full screen
- `Esc`: Exit full screen
- See `ZXCWASD.conf` for more shortcuts

## Quick Start

_Install `ffmpeg` first if you want video thumbnails to be generated._

1. Download a release
   - Download the archive or executable for your platform from the GitHub Releases page.
2. Extract and run
   - macOS / Linux: Open a terminal in the extracted directory and run `./echogallery`.
   - Windows: Double-click `echogallery.exe` or run it from Command Prompt.
3. Open the browser
   - Visit `http://127.0.0.1:8080` to open the EchoGallery setup page.
4. Complete the initial setup
   - Follow the prompts to create an administrator account.
   - Select or create a library folder for your media and library data.
5. Import a library
   - Select the folder containing your photos and videos as a library.
   - Wait for EchoGallery to scan and import the existing media.
6. Start browsing
   - Open Timeline, Full-size viewer, Albums, Favorites, or other views.

> On first launch, EchoGallery opens the main app when `config.json` and a database already exist. Otherwise, it opens the setup wizard.

## Installation

### Requirements

- Go 1.26+
- ffmpeg

### Build

```bash
git clone https://github.com/in4pira10n/EchoGallery.git
cd EchoGallery
go build -o echogallery .
```

### Run

```bash
./echogallery
```

On first run, if `config.json` is missing, EchoGallery automatically opens the setup page at:

If you are moving to another computer, use “Import transfer package” on the setup page. The package restores accounts, permissions, preferences, avatars, branding assets, and shortcuts. Copy the library folder together with its hidden `.echogallery` directory. If the disk path changes, choose the new path on the library recovery page after restarting.

The library database, thumbnails, playback cache, trash, logo, name, accent color, and stable ID are stored in `<library>/.echogallery/`. The Settings page can export an application configuration package. Runtime locks, page sessions, old caches, machine-specific cache paths, and local update paths are not included. Treat configuration packages as carefully as password backups because they contain password hashes.

```text
http://127.0.0.1:8080
```

## Acknowledgements

EchoGallery uses the following open-source projects:

- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [modernc/sqlite](https://github.com/modernc/sqlite)
- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)
- [DoYoungDo/PhotoAlbum](https://github.com/DoYoungDo/PhotoAlbum)
- And other open-source dependencies used by this project
