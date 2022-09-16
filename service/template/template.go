package template

import (
	"context"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"os"
	"strings"
	textTemplate "text/template"

	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Template is a templating service
	Template interface {
		Execute(dst io.Writer, kind Kind, templateName string, data interface{}) error
		ExecuteHTML(dst io.Writer, kind Kind, templateName string, data interface{}) error
	}

	// Service is a Template and governor.Service
	Service interface {
		governor.Service
		Template
	}

	service struct {
		tt  *textTemplate.Template
		ht  *htmlTemplate.Template
		log *klog.LevelLogger
	}

	ctxKeyTemplate struct{}
)

type (
	// Template source kind
	Kind string
)

const (
	// KindLocal indicates a local template
	KindLocal Kind = "local"
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

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxTemplate(inj, s)

	r.SetDefault("dir", "templates")
	r.SetDefault("txtglob", "*.txt")
	r.SetDefault("htmlglob", "*.html")
}

const (
	tplNoMatchErrSubstring = "pattern matches no files"
)

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	templateDir := os.DirFS(r.GetStr("dir"))
	tt, err := textTemplate.ParseFS(templateDir, r.GetStr("txtglob"))
	if err != nil {
		if strings.Contains(err.Error(), tplNoMatchErrSubstring) {
			s.log.Warn(ctx, "No txt templates loaded", klog.Fields{
				"tpl.local.txtfile.pattern": r.GetStr("txtglob"),
			})
			tt = textTemplate.New("default")
		} else {
			return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Failed to load templates")
		}
	}
	s.tt = tt
	ht, err := htmlTemplate.ParseFS(templateDir, r.GetStr("htmlglob"))
	if err != nil {
		if strings.Contains(err.Error(), tplNoMatchErrSubstring) {
			s.log.Warn(ctx, "No html templates loaded", klog.Fields{
				"tpl.local.htmlfile.pattern": r.GetStr("htmlglob"),
			})
			ht = htmlTemplate.New("default")
		} else {
			return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Failed to load templates")
		}
	}
	s.ht = ht

	if k := tt.DefinedTemplates(); k != "" {
		s.log.Info(ctx, "Loaded text templates", klog.Fields{
			"templates": strings.TrimLeft(k, "; "),
		})
	}
	if k := ht.DefinedTemplates(); k != "" {
		s.log.Info(ctx, "Loaded html templates", klog.Fields{
			"templates": strings.TrimLeft(k, "; "),
		})
	}
	return nil
}

func (s *service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health(ctx context.Context) error {
	return nil
}

type (
	// ErrorTemplateDNE is returned when a template does not exist
	ErrorTemplateDNE struct{}
	// ErrorExecute is returned when failing to execute a template
	ErrorExecute struct{}
)

func (e ErrorTemplateDNE) Error() string {
	return "Template does not exist"
}

func (e ErrorExecute) Error() string {
	return "Error executing template"
}

// Execute executes a template and returns the templated string
func (s *service) Execute(dst io.Writer, kind Kind, templateName string, data interface{}) error {
	switch kind {
	case KindLocal:
		if s.tt.Lookup(templateName) == nil {
			return kerrors.WithKind(nil, ErrorTemplateDNE{}, fmt.Sprintf("Template %s does not exist", templateName))
		}
		if err := s.tt.ExecuteTemplate(dst, templateName, data); err != nil {
			return kerrors.WithKind(err, ErrorExecute{}, "Failed executing text template")
		}
	default:
		return kerrors.WithKind(nil, ErrorTemplateDNE{}, "Invalid text template kind")
	}
	return nil
}

// ExecuteHTML executes an html template and returns the templated string
func (s *service) ExecuteHTML(dst io.Writer, kind Kind, templateName string, data interface{}) error {
	switch kind {
	case KindLocal:
		if s.ht.Lookup(templateName) == nil {
			return kerrors.WithKind(nil, ErrorTemplateDNE{}, fmt.Sprintf("Template %s does not exist", templateName))
		}
		if err := s.ht.ExecuteTemplate(dst, templateName, data); err != nil {
			return kerrors.WithKind(err, ErrorExecute{}, "Failed executing html template")
		}
	default:
		return kerrors.WithKind(nil, ErrorTemplateDNE{}, "Invalid html template kind")
	}
	return nil
}
