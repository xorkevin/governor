package user

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

func (u *userRouter) mountRest(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	if err := u.mountGet(conf, r); err != nil {
		return err
	}
	if err := u.mountSession(conf, r); err != nil {
		return err
	}
	if err := u.mountCreate(conf, r); err != nil {
		return err
	}
	if err := u.mountEdit(conf, r); err != nil {
		return err
	}
	if err := u.mountEditSecure(conf, r); err != nil {
		return err
	}
	return nil
}
