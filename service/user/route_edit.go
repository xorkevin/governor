package user

import (
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

type (
	//forge:valid
	reqUserPut struct {
		Username  string `valid:"username" json:"username"`
		FirstName string `valid:"firstName" json:"first_name"`
		LastName  string `valid:"lastName" json:"last_name"`
	}
)

func (s *router) putUser(c *governor.Context) {
	userid := gate.GetCtxUserid(c)

	var req reqUserPut
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.updateUser(c.Ctx(), userid, req); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqUserPatchRole struct {
		Userid string `valid:"userid,has" json:"-"`
		Role   string `valid:"role" json:"role"`
		Mod    bool   `json:"mod"`
		Add    bool   `json:"add"`
	}
)

func (s *router) patchRole(c *governor.Context) {
	var req reqUserPatchRole
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	updaterUserid := gate.GetCtxUserid(c)
	if err := s.s.updateRole(c.Ctx(), req.Userid, updaterUserid, req.Role, req.Mod, req.Add); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountEdit(m *governor.MethodRouter) {
	scopeAdminWrite := s.s.scopens + ".admin:write"
	m.PutCtx("", s.putUser, gate.AuthUserSudo(s.s.gate, s.s.authSettings.sudoDuration, gate.ScopeNone), s.rt)
	m.PatchCtx("/id/{id}/role", s.patchRole, gate.AuthUser(s.s.gate, scopeAdminWrite), s.rt)
}
