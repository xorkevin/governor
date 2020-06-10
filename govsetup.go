package governor

import (
	"github.com/labstack/echo/v4"
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
	responseSetup struct {
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Version   string `json:"version"`
	}
)

func (s *Server) initSetup(r *echo.Group) {
	r.POST("", func(c echo.Context) error {
		req := &ReqSetup{}
		if err := c.Bind(req); err != nil {
			return err
		}
		if err := req.valid(); err != nil {
			return err
		}
		if err := s.setupServices(*req); err != nil {
			return err
		}

		return c.JSON(http.StatusCreated, &responseSetup{
			Username:  req.Username,
			Firstname: req.Firstname,
			Lastname:  req.Lastname,
			Version:   s.config.version.Num,
		})
	})
}
