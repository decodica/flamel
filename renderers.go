package flamel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"html/template"
	"io"
	"net/http"
)

type Renderer interface {
	Render(w http.ResponseWriter) error
}

// Renders a GO HTML template
type TemplateRenderer struct {
	Template     *template.Template
	TemplateName string
	Data         interface{}
}

func (renderer *TemplateRenderer) Render(w http.ResponseWriter) error {
	buf := instance.bufferPool.Get().(bytes.Buffer)
	defer instance.bufferPool.Put(buf)
	err := renderer.Template.ExecuteTemplate(&buf, renderer.TemplateName, renderer.Data)
	if err != nil {
		return err
	}
	_, err = buf.WriteTo(w)
	return err
}

// Returns the data as JSON object(s)
type JSONRenderer struct {
	Data interface{}
}

func (renderer *JSONRenderer) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	return json.NewEncoder(w).Encode(renderer.Data)
}

// Renders plain text
type TextRenderer struct {
	Data string
}

func (renderer *TextRenderer) Render(w http.ResponseWriter) error {
	_, err := io.WriteString(w, renderer.Data)
	return err
}

// Renders a file as returned from the BlobStore
type BlobRenderer struct {
	Data appengine.BlobKey
}

func (renderer *BlobRenderer) Render(w http.ResponseWriter) error {
	blobstore.Send(w, renderer.Data)
	return nil
}

type ErrorRenderer struct {
	Data error
}

func (renderer *ErrorRenderer) Render(w http.ResponseWriter) error {
	_, err := io.WriteString(w, renderer.Data.Error())
	return err
}

type DownloadRenderer struct {
	Mime string
	Encoding string
	FileName string
	Data []byte
}

func (renderer *DownloadRenderer) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", renderer.FileName))
	if renderer.Mime == "" {
		w.Header().Set("Content-Type", "application/octet-stream; charset=UTF-8")
	} else {
		w.Header().Set("Content-Type", fmt.Sprintf("%s; charset=%s", renderer.Mime, renderer.Encoding))
	}
	_, err := w.Write(renderer.Data)
	return err
}
