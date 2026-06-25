package main

import (
	"fmt"
	"os"

	"echogallery/internal/update"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "apply-plan" {
		fmt.Fprintln(os.Stderr, "Usage: EchoGalleryUpdater apply-plan <plan.json>")
		os.Exit(2)
	}
	if err := update.RunApplyPlan(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "EchoGalleryUpdater failed: %v\n", err)
		os.Exit(1)
	}
}
