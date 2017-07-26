package post

import (
	"strings"
)

const (
	editEscapeSequence = "%edit%"
)

func assembleContent(original, updated string) string {
	return original + editEscapeSequence + updated
}

// returns the original then the updated and if the updated exists
func parseContent(stored string) (string, string, bool) {
	s := strings.SplitN(stored, editEscapeSequence, 2)
	if len(s) == 2 {
		return s[0], s[1], true
	}
	return s[0], "", false
}
