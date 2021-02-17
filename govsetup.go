package governor

import (
	"net/http"
	"regexp"
)

type (
	// ReqSetup is the option struct that is passed to all services during setup
	ReqSetup struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
	}
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *ReqSetup) valid() error {
	if len(r.Username) < 3 {
		return NewErrorUser("Username must be longer than 2 chars", http.StatusBadRequest, nil)
	}
	if len(r.Password) < 10 {
		return NewErrorUser("Password must be longer than 9 chars", http.StatusBadRequest, nil)
	}
	if !emailRegex.MatchString(r.Email) {
		return NewErrorUser("Email is invalid", http.StatusBadRequest, nil)
	}
	return nil
}

type (
	// ResponseSetup is the response to a setup request
	ResponseSetup struct {
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Version   string `json:"version"`
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
			Username:  req.Username,
			Firstname: req.Firstname,
			Lastname:  req.Lastname,
			Version:   s.config.version.Num,
		})
	})
}
