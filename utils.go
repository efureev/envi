package envi

import (
	"os"
	"strings"
)

func isExistPath(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}

func removeAdjacentDuplicates(s string) string {
	str := strings.Split(s, ``)
	str = removeAdjacentDups(str)

	return strings.Join(str, ``)
}

func removeAdjacentDuplicatesOnly(str, s string) string {
	strA := strings.Split(str, ``)
	strA = removeAdjacentDupsOnly(strA, s)

	return strings.Join(strA, ``)
}

func removeAdjacentDups(strings []string) []string {
	// Iterate over all characters in the slice except the last one
	for i := 0; i < len(strings)-1; {
		// Check whether the character next to it is a duplicate
		if strings[i] == strings[i+1] {
			// If it is, remove the CURRENT character from the slice
			strings = append(strings[:i], strings[i+1:]...)
		} else {
			// If it's not, move to the next item in the slice
			i++
		}
	}
	return strings
}

func removeAdjacentDupsOnly(strings []string, s string) []string {
	for i := 0; i < len(strings)-1; {
		if s != strings[i] {
			i++
			continue
		}

		if strings[i] == strings[i+1] {
			strings = append(strings[:i], strings[i+1:]...)
		} else {
			i++
		}
	}
	return strings
}
