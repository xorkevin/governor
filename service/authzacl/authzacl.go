package authzacl

import (
	"xorkevin.dev/governor"
)

type (
	ACL interface{}

	Service struct{}

	ctxKeyService struct{}
)

// GetCtxACL returns an ACL service from the context
func GetCtxACL(inj governor.Injector) ACL {
	v := inj.Get(ctxKeyService{})
	if v == nil {
		return nil
	}
	return v.(ACL)
}

// setCtxACL sets an ACL service in the context
func setCtxACL(inj governor.Injector, a ACL) {
	inj.Set(ctxKeyService{}, a)
}
