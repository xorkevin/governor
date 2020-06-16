package user

import (
	"xorkevin.dev/governor"
)

func (r *router) mountRoute(m governor.Router) {
	r.mountGet(m)
	r.mountSession(m)
	r.mountCreate(m)
	r.mountEdit(m)
	r.mountEditSecure(m)
}
