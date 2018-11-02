package template

import (
	"bytes"
	"fmt"
	"github.com/hackform/governor"
	htmlTemplate "html/template"
	"net/http"
	"strings"
)

const (
	moduleID = "template"
)

type (
	// Template is a templating service
	Template interface {
		Execute(templateName string, data interface{}) (string, *governor.Error)
		ExecuteHTML(filename string, data interface{}) (string, *governor.Error)
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
			l.Warn("template: no templates loaded", moduleID, "no templates loaded", 0, nil)
			t = htmlTemplate.New("default")
		} else {
			l.Error(fmt.Sprintf("error creating Template: %s", err), moduleID, "fail create template", 0, nil)
			return nil, err
		}
	}

	if k := t.DefinedTemplates(); k != "" {
		l.Info(fmt.Sprintf("template: %s", strings.TrimLeft(k, "; ")), moduleID, "load templates", 0, nil)
	}

	l.Info("initialized template service", moduleID, "initialize template service", 0, nil)

	return &templateService{
		t: t,
	}, nil
}

const (
	moduleIDExecute = moduleID + ".Execute"
)

// Execute executes a template and returns the templated string
func (t *templateService) Execute(templateName string, data interface{}) (string, *governor.Error) {
	b := bytes.Buffer{}
	if err := t.t.ExecuteTemplate(&b, templateName, data); err != nil {
		return "", governor.NewError(moduleIDExecute, err.Error(), 0, http.StatusInternalServerError)
	}
	return b.String(), nil
}

// ExecuteHTML executes an html file and returns the templated string
func (t *templateService) ExecuteHTML(filename string, data interface{}) (string, *governor.Error) {
	return t.Execute(filename+".html", data)
}
