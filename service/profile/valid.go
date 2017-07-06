package profile

import (
	"github.com/hackform/governor"
	"net/http"
)

const (
	moduleIDReqValid = moduleID + ".reqvalid"
	lengthCap        = 128
	lengthCapLarge   = 4096
)

func hasUserid(userid string) *governor.Error {
	if len(userid) < 1 || len(userid) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "userid must be provided", 0, http.StatusBadRequest)
	}
	return nil
}
