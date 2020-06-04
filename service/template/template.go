package template

import (
	"bytes"
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	htmlTemplate "html/template"
	"net/http"
	"strings"
	textTemplate "text/template"
	"xorkevin.dev/governor"
)

type (
	// Template is a templating service
	Template interface {
		Execute(templateName string, data interface{}) ([]byte, error)
		ExecuteHTML(filename string, data interface{}) ([]byte, error)
	}

	Service interface {
		governor.Service
		Template
	}

	service struct {
		tt     *textTemplate.Template
		ht     *htmlTemplate.Template
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
	templateDir := r.GetStr("dir")
	tt, err := textTemplate.ParseGlob(templateDir + "/*.txt")
	if err != nil {
		if err.Error() == fmt.Sprintf("text/template: pattern matches no files: %#q", r.GetStr("dir")+"/*.html") {
			l.Warn("no templates loaded", nil)
			tt = textTemplate.New("default")
		} else {
			return governor.NewError("Failed to load templates", http.StatusInternalServerError, err)
		}
	}
	s.tt = tt
	ht, err := htmlTemplate.ParseGlob(templateDir + "/*.html")
	if err != nil {
		if err.Error() == fmt.Sprintf("html/template: pattern matches no files: %#q", r.GetStr("dir")+"/*.html") {
			l.Warn("no templates loaded", nil)
			ht = htmlTemplate.New("default")
		} else {
			return governor.NewError("Failed to load templates", http.StatusInternalServerError, err)
		}
	}
	s.ht = ht

	if k := tt.DefinedTemplates(); k != "" {
		l.Info("loaded text templates", map[string]string{
			"templates": strings.TrimLeft(k, "; "),
		})
	}
	if k := ht.DefinedTemplates(); k != "" {
		l.Info("loaded html templates", map[string]string{
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
func (s *service) Execute(templateName string, data interface{}) ([]byte, error) {
	b := &bytes.Buffer{}
	if err := s.tt.ExecuteTemplate(b, templateName, data); err != nil {
		return nil, governor.NewError("Failed executing text template", http.StatusInternalServerError, err)
	}
	return b.Bytes(), nil
}

// ExecuteHTML executes an html template and returns the templated string
func (s *service) ExecuteHTML(templateName string, data interface{}) ([]byte, error) {
	b := &bytes.Buffer{}
	if err := s.ht.ExecuteTemplate(b, templateName, data); err != nil {
		return nil, governor.NewError("Failed executing html template", http.StatusInternalServerError, err)
	}
	return b.Bytes(), nil
}
