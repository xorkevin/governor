package rank

import (
	"sort"
	"strings"
)

const (
	tagUser  = "user"
	tagAdmin = "admin"
)

type (
	// Rank represents the set of all user auth tags
	Rank map[string]bool
)

// Stringify transforms the rank into an alphabetically ordered, comma delimited string
func (r Rank) Stringify() string {
	keys := []string{}
	for k, v := range r {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// BaseUser creates a new user rank
func BaseUser() Rank {
	return Rank{
		tagUser: true,
	}
}

// Admin creates a new Administrator rank
func Admin() Rank {
	b := BaseUser()
	b[tagAdmin] = true
	return b
}
