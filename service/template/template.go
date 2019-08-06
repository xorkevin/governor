package template

import (
	"bytes"
	"fmt"
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

	templateService struct {
		t *htmlTemplate.Template
	}
)

// New creates a new Template
func New(conf governor.Config, l governor.Logger) (Template, error) {
	t, err := htmlTemplate.ParseGlob(conf.TemplateDir + "/*.html")
	if err != nil {
		if err.Error() == fmt.Sprintf("html/template: pattern matches no files: %#q", conf.TemplateDir+"/*.html") {
			l.Warn("template: no templates loaded", nil)
			t = htmlTemplate.New("default")
		} else {
			l.Error(fmt.Sprintf("template: error creating Template: %s", err), nil)
			return nil, err
		}
	}

	if k := t.DefinedTemplates(); k != "" {
		l.Info(fmt.Sprintf("template: load templates: %s", strings.TrimLeft(k, "; ")), nil)
	}

	l.Info("initialize template service", nil)

	return &templateService{
		t: t,
	}, nil
}

// Execute executes a template and returns the templated string
func (t *templateService) Execute(templateName string, data interface{}) (string, error) {
	b := bytes.Buffer{}
	if err := t.t.ExecuteTemplate(&b, templateName, data); err != nil {
		return "", governor.NewError("Failed executing template", http.StatusInternalServerError, err)
	}
	return b.String(), nil
}

// ExecuteHTML executes an html file and returns the templated string
func (t *templateService) ExecuteHTML(filename string, data interface{}) (string, error) {
	return t.Execute(filename+".html", data)
}
