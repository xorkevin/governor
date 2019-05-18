package governor

import (
	"fmt"
	"github.com/labstack/echo"
	"golang.org/x/xerrors"
	"net/http"
	"regexp"
)

type (
	setup struct {
		services []Service
	}
)

// New creates a new Setup service
func newSetup() *setup {
	return &setup{
		services: []Service{},
	}
}

type (
	// ReqSetupPost is the http post request for the setup
	ReqSetupPost struct {
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

func (r *ReqSetupPost) valid() error {
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
	responseSetupPost struct {
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
		Version   string `json:"version"`
		Orgname   string `json:"orgname"`
	}

	goverrorSetup struct {
		message string
		status  int
		err     error
	}
)

func (e *goverrorSetup) Error() string {
	if e.err == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %s", e.message, e.err.Error())
}

func (e *goverrorSetup) Unwrap() error {
	return e.err
}

func (e *goverrorSetup) Is(target interface{}) bool {
	_, ok := target.(*goverrorSetup)
	return ok
}

func (e *goverrorSetup) As(target interface{}) bool {
	err, ok := target.(*goverrorSetup)
	if !ok {
		return false
	}
	err.message = e.message
	err.status = e.status
	err.err = e.err
	return true
}

var (
	setupRun = false
)

// Mount is a collection of routes
func (s *setup) Mount(conf Config, l Logger, r *echo.Group) error {
	r.POST("", func(c echo.Context) error {
		if setupRun {
			return NewErrorUser("setup already run", http.StatusForbidden, nil)
		}

		rsetup := &ReqSetupPost{}
		if err := c.Bind(rsetup); err != nil {
			return NewErrorUser("", http.StatusBadRequest, err)
		}
		if err := rsetup.valid(); err != nil {
			return err
		}

		for _, service := range s.services {
			if err := service.Setup(conf, l, *rsetup); err != nil {
				goverrsetup := goverrorSetup{}
				if xerrors.Is(err, &goverrsetup) {
					setupRun = true
					l.Warn("setup run again", nil)
					return NewErrorUser("setup already run", http.StatusForbidden, err)
				}
				return err
			}
		}

		l.Info("setup all services", nil)

		return c.JSON(http.StatusCreated, &responseSetupPost{
			Username:  rsetup.Username,
			Firstname: rsetup.Firstname,
			Lastname:  rsetup.Lastname,
			Version:   conf.Version,
			Orgname:   rsetup.Orgname,
		})
	})

	l.Info("mount setup service", nil)
	return nil
}

func (s *setup) addService(service Service) {
	s.services = append(s.services, service)
}
