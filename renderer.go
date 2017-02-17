package mage

import (
	"net/http"
	"html/template"
	"encoding/json"
	"io"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
)

type TemplateRenderer struct {
	Template template.Template
	TemplateName string
	Data interface{}
}

func (renderer *TemplateRenderer) Render(w http.ResponseWriter) error {
	return renderer.Template.ExecuteTemplate(w, renderer.TemplateName, renderer.Data);
}


type JSONRenderer struct {
	Data interface{}
}

func (renderer *JSONRenderer) Render(w http.ResponseWriter) error {
	return json.NewEncoder(w).Encode(renderer.Data);
}

type TextRenderer struct {
	Data string
}

func (renderer *TextRenderer) Render(w http.ResponseWriter) error {
	_, err := io.WriteString(w, renderer.Data);
	return err;
}

type BlobRenderer struct {
	Data appengine.BlobKey
}

func (renderer *BlobRenderer) Render(w http.ResponseWriter) error {
	blobstore.Send(w, renderer.Data);
	return nil;
}