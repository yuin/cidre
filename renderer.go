package cidre

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Renderer provides easy way to serialize objects and render template files.
type Renderer interface {
	// Compiles and caches template files
	Compile()
	// Renders a template file specified by the given name
	RenderTemplateFile(io.Writer, string, interface{})
	// Writes the contents and the Content-Type header to the http.ResponseWriter.
	Html(http.ResponseWriter, ...interface{})
	// Writes the contents and the Content-Type header to the http.ResponseWriter.
	Json(http.ResponseWriter, ...interface{})
	// Writes the contents and the Content-Type header to the http.ResponseWriter.
	Xml(http.ResponseWriter, ...interface{})
	// Writes the contents and the Content-Type header to the http.ResponseWriter.
	Text(http.ResponseWriter, ...interface{})
}

type BaseRenderer struct{}

// Json(w http.ResponseWriter, object interface{})
func (rndr *BaseRenderer) Json(w http.ResponseWriter, args ...interface{}) {
	if len(w.Header().Get("Content-Type")) == 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	obj := args[0]
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(obj); err != nil {
		panic(err)
	}
}

// Xml(w http.ResponseWriter, object interface{})
func (rndr *BaseRenderer) Xml(w http.ResponseWriter, args ...interface{}) {
	if len(w.Header().Get("Content-Type")) == 0 {
		w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
	}
	obj := args[0]
	encoder := xml.NewEncoder(w)
	if err := encoder.Encode(obj); err != nil {
		panic(err)
	}
}

// Text(w http.ResponseWriter, format string, formatargs ...interface{})
func (rndr *BaseRenderer) Text(w http.ResponseWriter, args ...interface{}) {
	if len(w.Header().Get("Content-Type")) == 0 {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	}
	format := args[0].(string)
	formatargs := args[1:len(args)]
	fmt.Fprintf(w, format, formatargs...)
}

// HtmlTemplateRendererConfig is a configuration object for the HtmlTemplateRenderer
type HtmlTemplateRendererConfig struct {
	TemplateDirectory string
	LeftDelim         string
	RightDelim        string
	FuncMap           template.FuncMap
}

// Returns a HtmlTemplateRendererConfig object that has default values set.
// If an 'init' function object argument is not nil, this function
// will call the function with the HtmlTemplateRendererConfig object.
func DefaultHtmlTemplateRendererConfig(init ...func(*HtmlTemplateRendererConfig)) *HtmlTemplateRendererConfig {
	rndr := &HtmlTemplateRendererConfig{
		TemplateDirectory: "",
		LeftDelim:         "{{",
		RightDelim:        "}}",
		FuncMap:           template.FuncMap{},
	}
	if len(init) > 0 {
		init[0](rndr)
	}
	return rndr
}

// Renderer interface implementation using an html/template module.
// HtmlTemplateRenderer loads files matches '*.tpl' recurcively.
//
//    ./templates
//     |
//     |- layout
//     |     |
//     |     |- main_layout.tpl
//     |     |- admin_layout.tpl
//     |
//     |- page1.tpl
//     |- page2.tpl
//
// HtmlTemplateRenderer supports layout by providing an `yield` pipeline.
//
// page1.tpl
//    {{/* extends main_layout */}}
//    <div>content</div>
//
// main_layout.tpl
//    <html><body>
//    {{ yield }}
//    </body></html>
//
// An `include` pileline is like an html/template's `template` pipeline, but
// it accepts "name" parameter dynamically.
//
// page1.tpl
//    <div>content</div>
//    {{ include .SubContents . }}
//
type HtmlTemplateRenderer struct {
	BaseRenderer
	Config    *HtmlTemplateRendererConfig
	templates map[string]*template.Template
	layouts   map[string]string
}

func NewHtmlTemplateRenderer(config *HtmlTemplateRendererConfig) *HtmlTemplateRenderer {
	rndr := &HtmlTemplateRenderer{
		Config:    config,
		templates: make(map[string]*template.Template),
		layouts:   make(map[string]string),
	}
	return rndr
}

func (rndr *HtmlTemplateRenderer) SetTemplate(name string, tpl *template.Template) {
	rndr.templates[name] = tpl
}

func (rndr *HtmlTemplateRenderer) GetTemplate(name string) (*template.Template, bool) {
	v, ok := rndr.templates[name]
	return v, ok
}

func (rndr *HtmlTemplateRenderer) SetLayout(name, layout string) {
	rndr.layouts[name] = layout
}

func (rndr *HtmlTemplateRenderer) GetLayout(name string) (string, bool) {
	v, ok := rndr.layouts[name]
	return v, ok
}

func (rndr *HtmlTemplateRenderer) Compile() {
	if len(rndr.Config.TemplateDirectory) == 0 {
		return
	}

	funcMap := template.FuncMap{
		"include": func(name string, param interface{}) template.HTML {
			var buf bytes.Buffer
			rndr.RenderTemplateFile(&buf, name, param)
			return template.HTML(buf.String())
		},
		"raw": func(h string) template.HTML { return template.HTML(h) },
		// parse time dummy function
		"yield": func() template.HTML { return template.HTML("") },
	}

	extendsReg := regexp.MustCompile(regexp.QuoteMeta(rndr.Config.LeftDelim) + `/\*\s*extends\s*([^\s]+)\s*\*/` + regexp.QuoteMeta(rndr.Config.RightDelim))
	filepath.Walk(rndr.Config.TemplateDirectory, func(path string, file os.FileInfo, err error) error {
		filename := filepath.Base(path)
		if err != nil || !strings.HasSuffix(filename, ".tpl") {
			return nil
		}
		tplname := filename[0 : len(filename)-len(".tpl")]
		bts, err1 := ioutil.ReadFile(path)
		if err1 != nil {
			panic(err1)
		}
		matches := extendsReg.FindAllSubmatch(bts, -1)
		if len(matches) > 0 {
			rndr.SetLayout(tplname, string(matches[0][1]))
		}
		tplobj, err2 := template.New("").Delims(rndr.Config.LeftDelim, rndr.Config.RightDelim).Funcs(rndr.Config.FuncMap).Funcs(funcMap).Parse(string(bts))
		if err2 != nil {
			panic(err2)
		}
		rndr.SetTemplate(tplname, tplobj)
		return nil
	})
}

func (rndr *HtmlTemplateRenderer) getTempalte(name string) *template.Template {
	tpl, ok := rndr.GetTemplate(name)
	if !ok {
		panic("template '" + name + "' not found.")
	}
	return tpl
}

func (rndr *HtmlTemplateRenderer) RenderTemplateFile(w io.Writer, name string, param interface{}) {
	tpl := rndr.getTempalte(name)
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, param); err != nil {
		panic(err)
	}
	layout, ok := rndr.GetLayout(name)
	if ok {
		laytoutpl, _ := rndr.getTempalte(layout).Clone()
		laytoutpl.Funcs(template.FuncMap{
			"yield": func() template.HTML {
				return template.HTML(buf.String())
			},
		})
		if err := laytoutpl.Execute(w, param); err != nil {
			panic(err)
		}
	} else {
		w.Write(buf.Bytes())
	}
}

func (rndr *HtmlTemplateRenderer) Html(w http.ResponseWriter, args ...interface{}) {
	if len(w.Header().Get("Content-Type")) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	}
	name := args[0].(string)
	param := args[1]
	rndr.RenderTemplateFile(w, name, param)
}
