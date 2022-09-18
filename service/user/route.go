package user

import (
	"xorkevin.dev/governor"
)

func (s *router) mountRoute(r governor.Router) {
	m := governor.NewMethodRouter(r)
	s.mountGet(m)
	s.mountSession(m)
	s.mountCreate(m)
	s.mountEdit(m)
	s.mountEditSecure(m)
}
