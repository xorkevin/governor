package rank

import (
	"github.com/hackform/governor"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const (
	moduleID = "rank"
)

// Tags for user rank
const (
	TagUser   = "user"
	TagAdmin  = "admin"
	TagSystem = "system"
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

// HasMod checks if a Rank has a moderator tag
func (r Rank) HasMod(tag string) bool {
	val, ok := r["mod_"+tag]
	return ok && val
}

// HasUser checks if a Rank has a user tag
func (r Rank) HasUser(tag string) bool {
	val, ok := r["user_"+tag]
	return ok && val
}

const (
	moduleIDFromString = moduleID + ".fromstring"
)

var (
	rankRegexMod  = regexp.MustCompile(`^mod_[a-z][a-z0-9.-]+$`)
	rankRegexUser = regexp.MustCompile(`^user_[a-z][a-z0-9.-]+$`)
)

// FromString creates a new Rank from a string
func FromString(rankString string) (Rank, *governor.Error) {
	r := Rank{}
	for _, i := range strings.Split(rankString, ",") {
		if !rankRegexMod.MatchString(i) && !rankRegexUser.MatchString(i) && i != TagUser && i != TagAdmin && i != TagSystem {
			return Rank{}, governor.NewError(moduleIDFromString, "illegal rank string", 0, http.StatusBadRequest)
		}
		r[i] = true
	}
	return r, nil
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
