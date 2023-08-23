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

	Service struct {
		tt  *textTemplate.Template
		ht  *htmlTemplate.Template
		log *klog.LevelLogger
	}
)

type (
	// Template source kind
	Kind string
)

const (
	// KindLocal indicates a local template
	KindLocal Kind = "local"
)

// New creates a new Template service
func New() *Service {
	return &Service{}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("dir", "templates")
	r.SetDefault("txtglob", "*.txt.tmpl")
	r.SetDefault("htmlglob", "*.html.tmpl")
}

const (
	tplNoMatchErrorSubstring = "pattern matches no files"
)

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	templateDir := os.DirFS(r.GetStr("dir"))
	tt, err := textTemplate.ParseFS(templateDir, r.GetStr("txtglob"))
	if err != nil {
		if strings.Contains(err.Error(), tplNoMatchErrorSubstring) {
			s.log.Warn(ctx, "No txt templates loaded",
				klog.AString("pattern", r.GetStr("txtglob")),
			)
			tt = textTemplate.New("default")
		} else {
			return kerrors.WithKind(err, governor.ErrInvalidConfig, "Failed to load templates")
		}
	}
	s.tt = tt
	ht, err := htmlTemplate.ParseFS(templateDir, r.GetStr("htmlglob"))
	if err != nil {
		if strings.Contains(err.Error(), tplNoMatchErrorSubstring) {
			s.log.Warn(ctx, "No html templates loaded",
				klog.AString("pattern", r.GetStr("htmlglob")),
			)
			ht = htmlTemplate.New("default")
		} else {
			return kerrors.WithKind(err, governor.ErrInvalidConfig, "Failed to load templates")
		}
	}
	s.ht = ht

	if k := tt.DefinedTemplates(); k != "" {
		s.log.Info(ctx, "Loaded text templates",
			klog.AString("templates", strings.TrimLeft(k, "; ")),
		)
	}
	if k := ht.DefinedTemplates(); k != "" {
		s.log.Info(ctx, "Loaded html templates",
			klog.AString("templates", strings.TrimLeft(k, "; ")),
		)
	}
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

// Template errors
var (
	// ErrTemplateDNE is returned when a template does not exist
	ErrTemplateDNE errTemplateDNE
	// ErrExecute is returned when failing to execute a template
	ErrExecute errExecute
)

type (
	errTemplateDNE struct{}
	errExecute     struct{}
)

func (e errTemplateDNE) Error() string {
	return "Template does not exist"
}

func (e errExecute) Error() string {
	return "Error executing template"
}

// Execute executes a template and returns the templated string
func (s *Service) Execute(dst io.Writer, kind Kind, templateName string, data interface{}) error {
	switch kind {
	case KindLocal:
		if s.tt.Lookup(templateName) == nil {
			return kerrors.WithKind(nil, ErrTemplateDNE, fmt.Sprintf("Template %s does not exist", templateName))
		}
		if err := s.tt.ExecuteTemplate(dst, templateName, data); err != nil {
			return kerrors.WithKind(err, ErrExecute, "Failed executing text template")
		}
	default:
		return kerrors.WithKind(nil, ErrTemplateDNE, "Invalid text template kind")
	}
	return nil
}

// ExecuteHTML executes an html template and returns the templated string
func (s *Service) ExecuteHTML(dst io.Writer, kind Kind, templateName string, data interface{}) error {
	switch kind {
	case KindLocal:
		if s.ht.Lookup(templateName) == nil {
			return kerrors.WithKind(nil, ErrTemplateDNE, fmt.Sprintf("Template %s does not exist", templateName))
		}
		if err := s.ht.ExecuteTemplate(dst, templateName, data); err != nil {
			return kerrors.WithKind(err, ErrExecute, "Failed executing html template")
		}
	default:
		return kerrors.WithKind(nil, ErrTemplateDNE, "Invalid html template kind")
	}
	return nil
}
