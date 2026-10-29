// echogallery-manifest creates release metadata after both executables build.
package main

import (
	"echogallery/internal/update"
	"flag"
	"log"
)

func main() {
	dir := flag.String("dir", ".", "directory containing both executables")
	version := flag.String("version", "", "semantic release version, e.g. v3.0.0")
	goos := flag.String("os", "", "target GOOS")
	arch := flag.String("arch", "", "target GOARCH")
	flag.Parse()
	if *goos != "windows" && *goos != "darwin" && *goos != "linux" {
		log.Fatal("unsupported operating system")
	}
	if *arch != "amd64" && *arch != "arm64" {
		log.Fatal("unsupported architecture")
	}
	if err := update.WriteManifest(*dir, *version, *goos, *arch); err != nil {
		log.Fatal(err)
	}
}
