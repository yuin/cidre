package cidre

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestAppAction(t *testing.T) {
	app := NewApp(DefaultAppConfig())
    app.Renderer = NewHtmlTemplateRenderer(DefaultHtmlTemplateRendererConfig())
	p1 := app.MountPoint("/p1")

	p1.Get("page1", "page1/(?P<param1>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		app.Renderer.Text(w, "value:%v", RequestContext(r).PathParams.Get("param1"))
	})

	req, _ := http.NewRequest("GET", "/p1/page1/value", nil)
	writer := httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "value:value", writer.Body.String())
	errorIfNotEqual(t, 200, writer.Code)
	errorIfNotEqual(t, "text/plain; charset=UTF-8", writer.Header().Get("Content-Type"))
}

func TestAppNotFound(t *testing.T) {
	app := NewApp(DefaultAppConfig())
	p1 := app.MountPoint("/p1")

	p1.Get("page1", "page1/(?P<param1>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "value:%v", RequestContext(r).PathParams.Get("param1"))
	})
	req, _ := http.NewRequest("GET", "/p2/page1/value", nil)
	writer := httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, 404, writer.Code)
	errorIfNotEqual(t, "404 page not found", strings.TrimSpace(writer.Body.String()))

	app.OnNotFound = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, "Oops!")
	}
	req, _ = http.NewRequest("GET", "/p2/page1/value", nil)
	writer = httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, 404, writer.Code)
	errorIfNotEqual(t, "Oops!", strings.TrimSpace(writer.Body.String()))
}

func TestAppPanic(t *testing.T) {
	app := NewApp(DefaultAppConfig())
	root := app.MountPoint("/")

	root.Get("page1", "page1", func(w http.ResponseWriter, r *http.Request) {
		panic("panic!")
	})
	req, _ := http.NewRequest("GET", "/page1", nil)
	writer := httptest.NewRecorder()
	app.Config.Debug = false
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, 500, writer.Code)
	errorIfNotEqual(t, "Internal Server Error", strings.TrimSpace(writer.Body.String()))

	req, _ = http.NewRequest("GET", "/page1", nil)
	writer = httptest.NewRecorder()
	app.Config.Debug = true
	app.ServeHTTP(writer, req)
	stackTopLine := strings.Split(writer.Body.String(), "\n")[2]
	if m, _ := regexp.MatchString(`^.*\.go:(\d+) \([a-z0-9]+\)$`, stackTopLine); !m {
		t.Error("DefaultOnPanic should print stack trace.")
	}

	app.OnPanic = func(w http.ResponseWriter, r *http.Request, recv interface{}) {
		w.WriteHeader(500)
		fmt.Fprint(w, "Oops!")
	}
	req, _ = http.NewRequest("GET", "/page1", nil)
	writer = httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "Oops!", writer.Body.String())
}

func TestAppBuildUrl(t *testing.T) {
	app := NewApp(DefaultAppConfig())
	root := app.MountPoint("/")
	root.Get("p1", "p1/(?P<param1>[^/]+)/(?P<param2>[^/]+)",
		func(w http.ResponseWriter, r *http.Request) {})
	root.Get("p2", "p2/(?P<aaa>[^/]+)/(?P<bbb>[^/]+)",
		func(w http.ResponseWriter, r *http.Request) {})

	errorIfNotEqual(t, app.BuildUrl("p1", "aaa", "bbb"), "/p1/aaa/bbb")
}

func TestAppMiddleware(t *testing.T) {
	testMd1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("md1-1"))
		RequestContext(r).MiddlewareChain.DoNext(w, r)
		w.Write([]byte("md1-2"))
	})
	testMd2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("md2-1"))
		RequestContext(r).MiddlewareChain.DoNext(w, r)
		w.Write([]byte("md2-2"))
	})
	testMd3 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("md3-1"))
		RequestContext(r).MiddlewareChain.DoNext(w, r)
		w.Write([]byte("md3-2"))
	}
	testMd4 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("md4-1"))
		RequestContext(r).MiddlewareChain.DoNext(w, r)
		w.Write([]byte("md4-2"))
	})
	app := NewApp(DefaultAppConfig())
	app.Use(testMd1, testMd3)
	p1 := app.MountPoint("/p1")
	p1.Use(testMd2, testMd4)
	p1.Get("page1", "page1", func(w http.ResponseWriter, r *http.Request) {}, testMd3)
	p1.Get("page2", "page2", func(w http.ResponseWriter, r *http.Request) {})

	p2 := app.MountPoint("/p2")
	p2.Get("page3", "page3", func(w http.ResponseWriter, r *http.Request) {})

	req, _ := http.NewRequest("GET", "/p1/page1", nil)
	writer := httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "md1-1md3-1md2-1md4-1md3-1md3-2md4-2md2-2md3-2md1-2", writer.Body.String())
	req, _ = http.NewRequest("GET", "/p1/page2", nil)
	writer = httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "md1-1md3-1md2-1md4-1md4-2md2-2md3-2md1-2", writer.Body.String())

	req, _ = http.NewRequest("GET", "/p2/page3", nil)
	writer = httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "md1-1md3-1md3-2md1-2", writer.Body.String())
}

func TestResponseWriterHooks(t *testing.T) {
	app := NewApp(DefaultAppConfig())
	p1 := app.MountPoint("/p1")

	result := ""
	p1.Get("page1", "page1/(?P<param1>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		w.(ResponseWriter).Hooks().Add("before_write_header", func(w http.ResponseWriter, r *http.Request, data interface{}) {
			result = result + "3"
		})
		w.(ResponseWriter).Hooks().Add("before_write_header", func(w http.ResponseWriter, r *http.Request, data interface{}) {
			result = result + "2"
		})
		w.(ResponseWriter).Hooks().Add("before_write_content", func(w http.ResponseWriter, r *http.Request, data interface{}) {
			result = result + "4"
		})
		result = "1"
		w.Write([]byte(""))
	})

	req, _ := http.NewRequest("GET", "/p1/page1/value", nil)
	writer := httptest.NewRecorder()
	app.ServeHTTP(writer, req)
	errorIfNotEqual(t, "1234", result)
}
