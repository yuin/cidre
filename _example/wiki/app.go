// cidre sample: simple wiki app
package main

import (
	"errors"
	"fmt"
	"github.com/yuin/cidre"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WikiConfig struct {
	SiteName        string
	SiteDescription string
	DataDirectory   string
}

var wikiConfig = &WikiConfig{
	SiteName:        "cidre Wiki",
	SiteDescription: "Simple wiki app written in cidre",
	DataDirectory:   "./data",
}

type View struct {
	Context *cidre.Context
	App     *cidre.App
	Config  *WikiConfig
	Title   string
	Data    interface{}
	Flashes map[string][]string
}

func NewView(w http.ResponseWriter, r *http.Request, title string, data interface{}) *View {
	ctx := cidre.RequestContext(r)
	self := &View{ctx, ctx.App, wikiConfig, title, data, ctx.Session.Flashes()}
	return self
}

type Article struct {
	Name      string
	Body      string
	UpdatedAt time.Time
}

func LoadArticle(file string) (*Article, error) {
	basename := filepath.Base(file)
    article := &Article{Name: basename[0 : len(basename)-len(".txt")], Body:""}
	if body, err := ioutil.ReadFile(file); err != nil {
		return article, errors.New("Error")
	} else {
		article.Body = string(body)
	}
	if finfo, err := os.Stat(file); os.IsNotExist(err) {
		return article, errors.New("NotFound")
	} else {
		article.UpdatedAt = finfo.ModTime()
	}
	return article, nil
}

type Articles []*Article /* implements sort.Interface */

func (self Articles) Len() int {
	return len(self)
}

func (self Articles) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

func (self Articles) Less(i, j int) bool {
	return self[i].UpdatedAt.Unix() > self[j].UpdatedAt.Unix()
}

func main() {
	// Load configurations
	appConfig := cidre.DefaultAppConfig()
	sessionConfig := cidre.DefaultSessionConfig()
	_, err := cidre.ParseIniFile("app.ini",
        // cidre
		cidre.ConfigMapping{"cidre", appConfig},
        // session middleware
		cidre.ConfigMapping{"session.base", sessionConfig},
        // this app
		cidre.ConfigMapping{"wiki", wikiConfig},
	)
	if err != nil {
		panic(err)
	}
    // Renderer configuration & view helper functions
    renderConfig := cidre.DefaultHtmlTemplateRendererConfig()
    renderConfig.TemplateDirectory = appConfig.TemplateDirectory
    renderConfig.FuncMap["nl2br"] = func(text string) template.HTML {
      return template.HTML(strings.Replace(text, "\n", "<br />", -1))
    }

	app := cidre.NewApp(appConfig)
    // Set our HTML renderer 
    app.Renderer = cidre.NewHtmlTemplateRenderer(renderConfig)
    // Use the session middleware for flash messaging
	app.Use(cidre.NewSessionMiddleware(app, sessionConfig, nil))
	root := app.MountPoint("/")

    // serve static files
	root.Static("statics", "statics", "./statics")

	root.Get("show_pages", "", func(w http.ResponseWriter, r *http.Request) {
		files, err := filepath.Glob(filepath.Join(wikiConfig.DataDirectory, "*.txt"))
        if err != nil {
          app.OnPanic(w, r, err)
        }
		articles := make(Articles, 0, len(files))
		for _, file := range files {
            article, _ := LoadArticle(file)
			articles = append(articles, article)
		}
		sort.Sort(articles)
		app.Renderer.Html(w, "show_pages", NewView(w, r, "List pages", articles))
	})

	root.Get("show_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx := cidre.RequestContext(r)
		name := strings.Replace(ctx.PathParams.Get("name"), "..", "", -1)
		file := filepath.Join(wikiConfig.DataDirectory, name+".txt")
		article, err := LoadArticle(file)
		if err != nil {
			switch err.Error() {
			case "NotFound":
				app.OnNotFound(w, r)
			default:
				app.OnPanic(w, r, err)
			}
			return
		}
		app.Renderer.Html(w, "show_page", NewView(w, r, "Page:"+name, article))
	})

	root.Get("edit_page", "pages/(?P<name>[^/]+)/edit", func(w http.ResponseWriter, r *http.Request) {
		ctx := cidre.RequestContext(r)
		name := strings.Replace(ctx.PathParams.Get("name"), "..", "", -1)
		file := filepath.Join(wikiConfig.DataDirectory, name+".txt")
		article, _ := LoadArticle(file)
		app.Renderer.Html(w, "edit_page", NewView(w, r, "EDIT: "+name, article))
	})

	root.Post("save_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx := cidre.RequestContext(r)
		name := strings.Replace(ctx.PathParams.Get("name"), "..", "", -1)
		body := r.FormValue("body")
		file := filepath.Join(wikiConfig.DataDirectory, name+".txt")
		if err := ioutil.WriteFile(file, []byte(body), 0644); err != nil {
			ctx.Session.AddFlash("error", "Failed to save a page: "+err.Error())
			http.Redirect(w, r, app.BuildUrl("edit_page", name), http.StatusFound)
		} else {
			ctx.Session.AddFlash("info", "Page updated")
			http.Redirect(w, r, app.BuildUrl("show_page", name), http.StatusFound)
		}
	})

	root.Delete("delete_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx := cidre.RequestContext(r)
		name := strings.Replace(ctx.PathParams.Get("name"), "..", "", -1)
		file := filepath.Join(wikiConfig.DataDirectory, name+".txt")
        if err := os.Remove(file); err != nil {
          app.OnPanic(w, r, err)
          return
        }
		ctx.Session.AddFlash("info", "Page deleted")
		http.Redirect(w, r, app.BuildUrl("show_pages"), http.StatusFound)
    })

	app.Hooks.Add("start_request", func(w http.ResponseWriter, r *http.Request, data interface{}) {
		w.Header().Add("X-Server", "Go")
	})
	app.OnNotFound = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Oops! Page not found.")
	}

	app.Run()
}
