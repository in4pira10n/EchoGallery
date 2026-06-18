package config

import "strings"

func BatchTaskStateIsEmpty(state BatchTaskState) bool {
	return !state.SelectionConfigured &&
		len(state.SelectedLibraryIDs) == 0 &&
		len(state.SelectedPaths) == 0 &&
		strings.TrimSpace(state.Status) == "" &&
		strings.TrimSpace(state.Message) == "" &&
		!state.MoveLegacyThumbnails &&
		!state.CleanThumbnailFiles &&
		!state.BuildPlaybackCaches &&
		strings.TrimSpace(state.CurrentLibraryID) == "" &&
		strings.TrimSpace(state.CurrentLibraryName) == "" &&
		strings.TrimSpace(state.CurrentLibraryPath) == "" &&
		state.CurrentLibraryIndex == 0 &&
		state.TotalLibraries == 0 &&
		state.CompletedLibraries == 0 &&
		state.FailedLibraries == 0 &&
		strings.TrimSpace(state.CurrentPhase) == "" &&
		state.CurrentDone == 0 &&
		state.CurrentTotal == 0 &&
		state.CurrentPercent == 0 &&
		!state.LowResourceMode &&
		!state.AggressiveMode &&
		!state.ExitAfterComplete &&
		strings.TrimSpace(state.StartedAt) == "" &&
		strings.TrimSpace(state.UpdatedAt) == "" &&
		strings.TrimSpace(state.FinishedAt) == "" &&
		strings.TrimSpace(state.Error) == "" &&
		len(state.Libraries) == 0
}

func CloneBatchTaskState(state BatchTaskState) BatchTaskState {
	next := state
	next.SelectedLibraryIDs = append([]string(nil), state.SelectedLibraryIDs...)
	next.SelectedPaths = append([]string(nil), state.SelectedPaths...)
	next.Libraries = append([]BatchTaskLibraryState(nil), state.Libraries...)
	return next
}
