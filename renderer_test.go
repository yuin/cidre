package cidre

import (
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type testRenderViewStruct struct {
  Value string
  Int   int
}

func TestRendererHtml(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	tpldir := filepath.Join(filepath.Dir(file), "testing")
	renderer := NewHtmlTemplateRenderer(DefaultHtmlTemplateRendererConfig(
		func(config *HtmlTemplateRendererConfig) {
			config.TemplateDirectory = tpldir
		}))
	renderer.Compile()
	writer := httptest.NewRecorder()
    renderer.Html(writer, "page1", &testRenderViewStruct{"V1",0})
    errorIfNotEqual(t, "HEADER\n\n<p>PAGE1:V1</p>\n<p>COMMON</p>\n\n\nFOOTER\n", writer.Body.String())
    errorIfNotEqual(t, "text/html; charset=UTF-8", writer.Header().Get("Content-type"))

	writer = httptest.NewRecorder()
    renderer.Html(writer, "page2", &testRenderViewStruct{"V1",0})
    errorIfNotEqual(t, "PAGE2:V1\n", writer.Body.String())
}

func TestRendererJsonAndXml(t *testing.T) {
	renderer := NewHtmlTemplateRenderer(DefaultHtmlTemplateRendererConfig())
	writer := httptest.NewRecorder()
    renderer.Json(writer, &testRenderViewStruct{"ABCDE", 10})
    errorIfNotEqual(t, `{"Value":"ABCDE","Int":10}`, strings.TrimSpace(writer.Body.String()))
    errorIfNotEqual(t, "application/json", writer.Header().Get("Content-type"))

	writer = httptest.NewRecorder()
    renderer.Xml(writer, &testRenderViewStruct{"ABCDE", 10})
    errorIfNotEqual(t, `<testRenderViewStruct><Value>ABCDE</Value><Int>10</Int></testRenderViewStruct>`, strings.TrimSpace(writer.Body.String()))
    errorIfNotEqual(t, "application/xml; charset=UTF-8", writer.Header().Get("Content-type"))
}
