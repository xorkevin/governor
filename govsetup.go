package governor

import (
	"github.com/labstack/echo"
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
		Orgname   string `json:"orgname"`
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
	if len(r.Orgname) == 0 {
		return NewErrorUser("organization name must be provided", http.StatusBadRequest, nil)
	}
	return nil
}

type (
	responseSetup struct {
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Version   string `json:"version"`
		Orgname   string `json:"orgname"`
	}
)

func (s *Server) initSetup(r *echo.Group) {
	r.POST("", func(c echo.Context) error {
		rsetup := &ReqSetup{}
		if err := c.Bind(rsetup); err != nil {
			return err
		}
		if err := rsetup.valid(); err != nil {
			return err
		}
		if err := s.setupServices(*rsetup); err != nil {
			return err
		}

		return c.JSON(http.StatusCreated, &responseSetup{
			Username:  rsetup.Username,
			Firstname: rsetup.Firstname,
			Lastname:  rsetup.Lastname,
			Version:   s.config.Version,
			Orgname:   rsetup.Orgname,
		})
	})
}
