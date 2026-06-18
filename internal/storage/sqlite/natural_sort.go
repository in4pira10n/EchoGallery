package sqlite

import (
	"sort"
	"strings"
	"unicode"

	"echogallery/internal/storage"
)

func sortPhotosByNaturalName(photos []*storage.Photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		cmp := compareNaturalStrings(photos[i].OriginalName, photos[j].OriginalName)
		if cmp != 0 {
			return cmp < 0
		}
		return photos[i].ID < photos[j].ID
	})
}

func compareNaturalStrings(a, b string) int {
	ar := []rune(strings.ToLower(a))
	br := []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			ai, aj := digitRun(ar, i)
			bi, bj := digitRun(br, j)
			if cmp := compareDigitRuns(ar[ai:aj], br[bi:bj]); cmp != 0 {
				return cmp
			}
			i, j = aj, bj
			continue
		}
		if ar[i] < br[j] {
			return -1
		}
		if ar[i] > br[j] {
			return 1
		}
		i++
		j++
	}
	if i < len(ar) {
		return 1
	}
	if j < len(br) {
		return -1
	}
	return 0
}

func digitRun(values []rune, start int) (int, int) {
	end := start
	for end < len(values) && unicode.IsDigit(values[end]) {
		end++
	}
	return start, end
}

func compareDigitRuns(a, b []rune) int {
	aTrimmed := trimLeadingZeroDigits(a)
	bTrimmed := trimLeadingZeroDigits(b)
	if len(aTrimmed) != len(bTrimmed) {
		if len(aTrimmed) < len(bTrimmed) {
			return -1
		}
		return 1
	}
	for i := range aTrimmed {
		if aTrimmed[i] < bTrimmed[i] {
			return -1
		}
		if aTrimmed[i] > bTrimmed[i] {
			return 1
		}
	}
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return 0
}

func trimLeadingZeroDigits(values []rune) []rune {
	index := 0
	for index < len(values)-1 && values[index] == '0' {
		index++
	}
	return values[index:]
}
