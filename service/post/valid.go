package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/rank"
	"net/http"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
)

func hasPostid(postid string) *governor.Error {
	if len(postid) < 1 || len(postid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "postid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func validContent(content string) *governor.Error {
	if len(content) == 0 {
		return governor.NewErrorUser(moduleIDReqValid, "content must be provided", 0, http.StatusBadRequest)
	}
	if len(content) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "content exceeds max length", 0, http.StatusBadRequest)
	}
	return nil
}

func validRank(rankString string) *governor.Error {
	if len(rankString) > lengthCapLarge {
		return governor.NewErrorUser(moduleIDReqValid, "rank exceeds the max length", 0, http.StatusBadRequest)
	}
	if _, err := rank.FromStringGroup(rankString); err != nil {
		return err
	}
	return nil
}
