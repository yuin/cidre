// cidre sample: simple wiki app
package main

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yuin/cidre"
	"html/template"
	"net/http"
	"path/filepath"
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
	Id        int
	Name      string
	Body      string
	UpdatedAt time.Time
	CreatedAt time.Time
}

// gorm.DB is thread safe
var DB gorm.DB

func InitDB(dataFilePath string) error {
	var err error
	DB, err = gorm.Open("sqlite3", dataFilePath)
	if err != nil {
		return err
	}
	if !DB.HasTable(Article{}) {
		DB.CreateTable(Article{})
		articleModel := DB.Model(Article{})
		articleModel.AddIndex("articles_name_idx", "name")
		articleModel.AddIndex("articles_updated_at_idx", "updated_at")
	}
	return nil
}

func FindArticle(db *gorm.DB, name string) (*Article, error) {
	var article Article
	err := db.Where("name = ?", name).First(&article).Error
	return &article, err
}

func FindArticles(db *gorm.DB) []Article {
	articles := make([]Article, 0, 10)
	db.Order("updated_at desc").Find(&articles)
	return articles
}

func DBTransactionMiddleware(w http.ResponseWriter, r *http.Request) {
	ctx := cidre.RequestContext(r)
	ctx.Set("db", DB.Begin())
	defer func() {
		status := w.(cidre.ResponseWriter).Status()
		if status >= 200 && status < 400 {
			ctx.Get("db").(*gorm.DB).Commit()
		} else {
			ctx.Get("db").(*gorm.DB).Rollback()
		}
	}()
	ctx.MiddlewareChain.DoNext(w, r)
}

func ctxdb(r *http.Request) (*cidre.Context, *gorm.DB) {
	ctx := cidre.RequestContext(r)
	db := ctx.Get("db").(*gorm.DB)
    return ctx, db
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
	app := cidre.NewApp(appConfig)
	// Set template function on setup
	app.Hooks.Add("setup", func(w http.ResponseWriter, r *http.Request, data interface{}) {
		config := app.Renderer.(*cidre.HtmlTemplateRenderer).Config
		config.FuncMap["nl2br"] = func(text string) template.HTML {
			return template.HTML(strings.Replace(text, "\n", "<br />", -1))
		}
	})

	// Auto transaction management
	app.Use(DBTransactionMiddleware)
	// Use the session middleware for flash messaging
	app.Use(cidre.NewSessionMiddleware(app, sessionConfig, nil))

	root := app.MountPoint("/")
	// serve static files
	root.Static("statics", "statics", "./statics")

	root.Get("show_pages", "", func(w http.ResponseWriter, r *http.Request) {
		_, db := ctxdb(r)
		app.Renderer.Html(w, "show_pages", NewView(w, r, "List pages", FindArticles(db)))
	})

	root.Get("show_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx, db := ctxdb(r)
		name := ctx.PathParams.Get("name")
		article, err := FindArticle(db, name)
		if err != nil {
			switch err {
			case gorm.RecordNotFound:
				app.OnNotFound(w, r)
			default:
				app.OnPanic(w, r, err)
			}
			return
		}
		app.Renderer.Html(w, "show_page", NewView(w, r, "Page:"+name, article))
	})

	root.Get("edit_page", "pages/(?P<name>[^/]+)/edit", func(w http.ResponseWriter, r *http.Request) {
		ctx, db := ctxdb(r)
		name := ctx.PathParams.Get("name")
		article, _ := FindArticle(db, name)
		article.Name = name
		app.Renderer.Html(w, "edit_page", NewView(w, r, "EDIT: "+name, article))
	})

	root.Post("save_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx, db := ctxdb(r)
		name := ctx.PathParams.Get("name")
		article, _ := FindArticle(db, name)
		article.Name = name
		article.Body = r.FormValue("body")
		if db.Save(article).Error != nil {
			ctx.Session.AddFlash("error", "Failed to save a page: "+err.Error())
			http.Redirect(w, r, app.BuildUrl("edit_page", name), http.StatusFound)
		} else {
			ctx.Session.AddFlash("info", "Page updated")
			http.Redirect(w, r, app.BuildUrl("show_page", name), http.StatusFound)
		}
	})

	root.Delete("delete_page", "pages/(?P<name>[^/]+)", func(w http.ResponseWriter, r *http.Request) {
		ctx, db := ctxdb(r)
		name := ctx.PathParams.Get("name")
		article, err := FindArticle(db, name)
		if err != nil && db.Delete(article).Error == nil {
			ctx.Session.AddFlash("info", "Page deleted")
			http.Redirect(w, r, app.BuildUrl("show_pages"), http.StatusFound)
		} else {
			ctx.Session.AddFlash("error", "Failed to delete a page: "+err.Error())
			http.Redirect(w, r, app.BuildUrl("edit_page", name), http.StatusFound)
		}
	})

	app.Hooks.Add("start_request", func(w http.ResponseWriter, r *http.Request, data interface{}) {
		w.Header().Add("X-Server", "Go")
	})
	app.OnNotFound = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Oops! Page not found.")
	}

	InitDB(filepath.Join(wikiConfig.DataDirectory, "wiki.bin"))
	app.Run()
}
