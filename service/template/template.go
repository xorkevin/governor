package template

import (
	"bytes"
	"context"
	"fmt"
	htmlTemplate "html/template"
	"os"
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

	// Service is a Template and governor.Service
	Service interface {
		governor.Service
		Template
	}

	service struct {
		tt     *textTemplate.Template
		ht     *htmlTemplate.Template
		logger governor.Logger
	}

	ctxKeyTemplate struct{}
)

// GetCtxTemplate returns a Template service from the context
func GetCtxTemplate(inj governor.Injector) Template {
	v := inj.Get(ctxKeyTemplate{})
	if v == nil {
		return nil
	}
	return v.(Template)
}

// setCtxTemplate sets a Template service in the context
func setCtxTemplate(inj governor.Injector, t Template) {
	inj.Set(ctxKeyTemplate{}, t)
}

// New creates a new Template service
func New() Service {
	return &service{}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxTemplate(inj, s)

	r.SetDefault("dir", "templates")
	r.SetDefault("txtglob", "*.txt")
	r.SetDefault("htmlglob", "*.html")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
	templateDir := os.DirFS(r.GetStr("dir"))
	tt, err := textTemplate.ParseFS(templateDir, r.GetStr("txtglob"))
	if err != nil {
		if err.Error() == fmt.Sprintf("text/template: pattern matches no files: %#q", r.GetStr("dir")+"/*.html") {
			l.Warn("no templates loaded", nil)
			tt = textTemplate.New("default")
		} else {
			return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Failed to load templates")
		}
	}
	s.tt = tt
	ht, err := htmlTemplate.ParseFS(templateDir, r.GetStr("htmlglob"))
	if err != nil {
		if err.Error() == fmt.Sprintf("html/template: pattern matches no files: %#q", r.GetStr("dir")+"/*.html") {
			l.Warn("no templates loaded", nil)
			ht = htmlTemplate.New("default")
		} else {
			return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Failed to load templates")
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

type (
	// ErrExecute is returned when failing to execute a template
	ErrExecute struct{}
)

func (e ErrExecute) Error() string {
	return "Error executing template"
}

// Execute executes a template and returns the templated string
func (s *service) Execute(templateName string, data interface{}) ([]byte, error) {
	b := &bytes.Buffer{}
	if err := s.tt.ExecuteTemplate(b, templateName, data); err != nil {
		return nil, governor.ErrWithKind(err, ErrExecute{}, "Failed executing text template")
	}
	return b.Bytes(), nil
}

// ExecuteHTML executes an html template and returns the templated string
func (s *service) ExecuteHTML(templateName string, data interface{}) ([]byte, error) {
	b := &bytes.Buffer{}
	if err := s.ht.ExecuteTemplate(b, templateName, data); err != nil {
		return nil, governor.ErrWithKind(err, ErrExecute{}, "Failed executing html template")
	}
	return b.Bytes(), nil
}
