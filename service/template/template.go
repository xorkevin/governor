package template

import (
	"bytes"
	"github.com/hackform/governor"
	"github.com/sirupsen/logrus"
	htmlTemplate "html/template"
	"net/http"
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
func New(conf governor.Config, l *logrus.Logger) (Template, error) {

	t, err := htmlTemplate.ParseGlob(conf.TemplateDir + "/*.html")

	if err != nil {
		return nil, err
	}

	l.Info("initialized template service")

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
