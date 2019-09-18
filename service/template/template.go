package template

import (
	"bytes"
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	htmlTemplate "html/template"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
)

type (
	// Template is a templating service
	Template interface {
		Execute(templateName string, data interface{}) (string, error)
		ExecuteHTML(filename string, data interface{}) (string, error)
	}

	Service interface {
		governor.Service
		Template
	}

	service struct {
		t      *htmlTemplate.Template
		logger governor.Logger
	}
)

// New creates a new Template service
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("dir", "templates")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
	t, err := htmlTemplate.ParseGlob(r.GetStr("dir") + "/*.html")
	if err != nil {
		if err.Error() == fmt.Sprintf("html/template: pattern matches no files: %#q", r.GetStr("dir")+"/*.html") {
			l.Warn("no templates loaded", nil)
			t = htmlTemplate.New("default")
		} else {
			l.Error("failed to load templates", map[string]string{
				"error": err.Error(),
			})
			return governor.NewError("Failed to load templates", http.StatusInternalServerError, err)
		}
	}

	s.t = t

	if k := t.DefinedTemplates(); k != "" {
		l.Info("loaded templates", map[string]string{
			"templates": strings.TrimLeft(k, "; "),
		})
	}
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// Execute executes a template and returns the templated string
func (s *service) Execute(templateName string, data interface{}) (string, error) {
	b := bytes.Buffer{}
	if err := s.t.ExecuteTemplate(&b, templateName, data); err != nil {
		return "", governor.NewError("Failed executing template", http.StatusInternalServerError, err)
	}
	return b.String(), nil
}

// ExecuteHTML executes an html file and returns the templated string
func (s *service) ExecuteHTML(filename string, data interface{}) (string, error) {
	return s.Execute(filename+".html", data)
}
