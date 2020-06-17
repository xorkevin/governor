package user

import (
	"xorkevin.dev/governor"
)

func (m *router) mountRoute(r governor.Router) {
	m.mountGet(r)
	m.mountSession(r)
	m.mountCreate(r)
	m.mountEdit(r)
	m.mountEditSecure(r)
}
