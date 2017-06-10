package rank

import (
	"sort"
	"strings"
)

// Tags for user rank
const (
	TagUser   = "user"
	TagAdmin  = "admin"
	TagSystem = "admin"
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

// Has checks if a Rank has a tag
func (r Rank) Has(tag string) bool {
	val, ok := r[tag]
	return ok && val
}

// FromString creates a new Rank from a string
func FromString(rankString string) Rank {
	r := Rank{}
	for _, i := range strings.Split(rankString, ",") {
		r[i] = true
	}
	return r
}

// BaseUser creates a new user rank
func BaseUser() Rank {
	return Rank{
		TagUser: true,
	}
}

// Admin creates a new Administrator rank
func Admin() Rank {
	b := BaseUser()
	b[TagAdmin] = true
	return b
}

// System creates a new System rank
func System() Rank {
	return Rank{
		TagSystem: true,
	}
}
