#!/bin/sh
# Build only by default. Archives require --package.
set -eu
cd "$(dirname "$0")/.."
build_os=$(go env GOOS)
build_arch=$(go env GOARCH)
build_version=v3.0.0
build_package=false
for arg in "$@"; do
 case "$arg" in
  --package) build_package=true ;;
  windows|darwin|linux) build_os=$arg ;;
  macos) build_os=darwin ;;
  amd|amd64|x64) build_arch=amd64 ;;
  arm|arm64) build_arch=arm64 ;;
  v[0-9]*.[0-9]*.[0-9]*) build_version=$arg ;;
  *) printf 'Unknown argument: %s\n' "$arg" >&2; exit 1 ;;
 esac
done
build_code="${build_version}build$(TZ=Asia/Shanghai date +%Y%m%d%H%M)"
build_root=".build"
rm -rf "$build_root"
build_dir="$build_root/$build_os-$build_arch"
mkdir -p "$build_dir"
build_suffix=
build_updater_flags="-X echogallery/internal/update.Version=$build_version -X echogallery/internal/update.VersionCode=$build_code"
if [ "$build_os" = windows ]; then
 build_suffix=.exe
 build_updater_flags="$build_updater_flags -H=windowsgui"
fi
GOOS="$build_os" GOARCH="$build_arch" go build -ldflags="-X echogallery/internal/update.Version=$build_version -X echogallery/internal/update.VersionCode=$build_code" -o "$build_dir/EchoGallery$build_suffix" .
GOOS="$build_os" GOARCH="$build_arch" go build -ldflags="$build_updater_flags" -o "$build_dir/EchoGalleryUpdater$build_suffix" ./cmd/echogallery-updater
if [ "$build_version" != dev ]; then
 go run ./cmd/echogallery-manifest -dir "$build_dir" -version "$build_version" -os "$build_os" -arch "$build_arch"
fi
printf 'Built in %s\n' "$build_dir"
if [ "$build_package" = true ]; then
 if [ "$build_version" = dev ]; then printf 'Packaging requires an explicit release version (vX.Y.Z).\n' >&2; exit 1; fi
	build_archive="EchoGallery_${build_os}_${build_arch}_${build_code}"
 if [ "$build_os" = windows ]; then
  (cd "$build_dir" && zip -q "$build_archive.zip" "EchoGallery$build_suffix" "EchoGalleryUpdater$build_suffix" manifest.json)
 else
  tar -C "$build_dir" -czf "$build_dir/$build_archive.tar.gz" "EchoGallery$build_suffix" "EchoGalleryUpdater$build_suffix" manifest.json
 fi
 printf 'Packaged in %s\n' "$build_dir"
fi
