package courier

import (
	"net/http"
	"net/url"
	"regexp"

	"xorkevin.dev/governor"
)

const (
	lengthCapUserid = 31
	lengthCap       = 63
	lengthCapURL    = 2047
	amountCap       = 1024
)

var (
	linkRegex = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func validLinkID(linkid string) error {
	if len(linkid) == 0 {
		return nil
	}
	if len(linkid) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Link id must be longer than 2 characters",
		}))
	}
	if len(linkid) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Link id must be shorter than 64 characters",
		}))
	}
	if !linkRegex.MatchString(linkid) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Link id can only contain A-Z,a-z,0-9,_,-",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasLinkID(linkid string) error {
	if len(linkid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Link id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(linkid) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Link id must be shorter than 64 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validBrandID(brandid string) error {
	if len(brandid) == 0 {
		return nil
	}
	if len(brandid) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Brand id must be longer than 2 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(brandid) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Brand id must be shorter than 64 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if !linkRegex.MatchString(brandid) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Brand id can only contain a-z,0-9,_,-",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasBrandID(brandid string) error {
	if len(brandid) == 0 {
		return nil
	}
	if len(brandid) > lengthCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Brand id must be shorter than 64 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validURL(rawurl string) error {
	if len(rawurl) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Url must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(rawurl) > lengthCapURL {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Url must be shorter than 2048 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	if u, err := url.Parse(rawurl); err != nil || !u.IsAbs() {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Url is invalid",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validhasCreatorID(creatorid string) error {
	if len(creatorid) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Creator id must be provided",
			Status:  http.StatusBadRequest,
		}))
	}
	if len(creatorid) > lengthCapUserid {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Creator id must be shorter than 32 characters",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validAmount(amt int) error {
	if amt == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be positive",
			Status:  http.StatusBadRequest,
		}))
	}
	if amt > amountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Amount must be less than 1024",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}

func validOffset(offset int) error {
	if offset < 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Message: "Offset must not be negative",
			Status:  http.StatusBadRequest,
		}))
	}
	return nil
}
