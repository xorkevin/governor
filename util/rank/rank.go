package rank

import (
	"github.com/hackform/governor"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const (
	rankLengthCap = 128
)

// Tags for user rank
const (
	TagUser       = "user"
	TagUserPrefix = "usr"
	TagBan        = "ban"
	TagBanPrefix  = "ban"
	TagMod        = "mod"
	TagModPrefix  = "mod"
	TagAdmin      = "admin"
	TagSystem     = "system"
)

type (
	// Rank represents the set of all user auth tags
	Rank map[string]bool
)

// Stringify transforms the rank into an alphabetically ordered, comma delimited string
func (r Rank) Stringify() string {
	keys := make([]string, 0, len(r))
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
	val, ok := r[TagMod+"_"+tag]
	return ok && val
}

// HasUser checks if a Rank has a user tag
func (r Rank) HasUser(tag string) bool {
	val, ok := r[TagUser+"_"+tag]
	return ok && val
}

// HasBan checks if a Rank has a ban tag
func (r Rank) HasBan(tag string) bool {
	val, ok := r[TagBan+"_"+tag]
	return ok && val
}

// Add adds a rank
func (r Rank) Add(other Rank) {
	for key, value := range other {
		if value {
			r[key] = true
		}
	}
}

// Remove removes a rank
func (r Rank) Remove(other Rank) {
	for key, value := range other {
		if value {
			delete(r, key)
		}
	}
}

var (
	rankRegexMod   = regexp.MustCompile(`^mod_[a-z][a-z0-9.-_]+$`)
	rankRegexUser  = regexp.MustCompile(`^usr_[a-z][a-z0-9.-_]+$`)
	rankRegexBan   = regexp.MustCompile(`^ban_[a-z][a-z0-9.-_]+$`)
	rankRegexGroup = regexp.MustCompile(`^grp_[a-z][a-z0-9.-_]+$`)
)

// FromStringUser creates a new User Rank from a string
func FromStringUser(rankString string) (Rank, error) {
	if len(rankString) < 1 {
		return Rank{}, nil
	}
	rankArray := strings.Split(rankString, ",")
	r := make(Rank, len(rankArray))
	for _, i := range rankArray {
		if len(i) > rankLengthCap || !rankRegexMod.MatchString(i) && !rankRegexUser.MatchString(i) && !rankRegexBan.MatchString(i) && i != TagUser && i != TagAdmin && i != TagSystem {
			return Rank{}, governor.NewErrorUser("Illegal rank string", http.StatusBadRequest, nil)
		}
		r[i] = true
	}
	return r, nil
}

// FromStringGroup creates a new Group Rank from a string
func FromStringGroup(rankString string) (Rank, error) {
	if len(rankString) < 1 {
		return Rank{}, nil
	}
	rankArray := strings.Split(rankString, ",")
	r := make(Rank, len(rankArray))
	for _, i := range rankArray {
		if !rankRegexGroup.MatchString(i) {
			return Rank{}, governor.NewErrorUser("Illegal rank string", http.StatusBadRequest, nil)
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
