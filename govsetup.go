package governor

import (
	"net/http"
	"regexp"
)

type (
	// SetupAdmin is the admin setup option
	SetupAdmin struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
	}

	// ReqSetup is the option struct that is passed to all services during setup
	ReqSetup struct {
		First  bool        `json:"first"`
		Secret string      `json:"secret,omitempty"`
		Admin  *SetupAdmin `json:"admin,omitempty"`
	}
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *ReqSetup) valid() error {
	if a := r.Admin; a != nil {
		if len(a.Username) < 3 {
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Username must be longer than 2 chars",
			}))
		}
		if len(a.Password) < 10 {
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Password must be longer than 9 chars",
			}))
		}
		if !emailRegex.MatchString(a.Email) {
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Email is invalid",
			}))
		}
	}
	return nil
}

type (
	// ResponseSetup is the response to a setup request
	ResponseSetup struct {
		Version string `json:"version"`
	}
)

func (s *Server) initSetup(m Router) {
	m.Post("", func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.logger)
		req := &ReqSetup{}
		if err := c.Bind(req); err != nil {
			c.WriteError(err)
			return
		}
		if err := req.valid(); err != nil {
			c.WriteError(err)
			return
		}
		if err := s.setupServices(*req); err != nil {
			c.WriteError(err)
			return
		}

		c.WriteJSON(http.StatusCreated, &ResponseSetup{
			Version: s.config.version.Num,
		})
	})
}
