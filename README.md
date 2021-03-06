# cidre [![GoDoc](https://godoc.org/github.com/yuin/cidre?status.svg)](http://godoc.org/github.com/yuin/cidre)

cidre is a modular and extensible thin web framework in Go.

~~~ go
package main

import (
  "github.com/yuin/cidre"
  "net/http"
)

func main() {
    app := cidre.NewApp(cidre.DefaultAppConfig())
    root := app.MountPoint("/")

    root.Get("show_welcome", "wellcome", func(w http.ResponseWriter, r *http.Request) {
        app.Renderer.Text(w, "Welcome!")
    })

    app.Run()
}
~~~

## How to Install

~~~
go get github.com/yuin/cidre
~~~

## Routing

~~~ go
app := cidre.NewApp(cidre.DefaultAppConfig())
app.Use(cidre.NewSessionMiddleware(app, sessionConfig, nil))

root := app.MountPoint("/")
root.Get("show_profile", "users/(?P<name>\w+)", func(w http.ResponseWriter, r *http.Request) {
  ctx := cidre.RequestContext(r)
  ctx.PathParams.Get("name")
  app.BuildUrl("show_profile", "alice") 
  // -> /users/alice
})

~~~

## Middleware

~~~ go
app := cidre.NewApp(cidre.DefaultAppConfig())
app.Use(cidre.NewSessionMiddleware(app, sessionConfig, nil))

root := app.MountPoint("/")
root.Use(func(w http.ResponseWriter, r *http.Request){
  // do something

  cidre.RequestContext(r).MiddlewareChain.DoNext(w,r)

  // do something
})
~~~

## HTML rendering

~~~ go
appconfig := cidre.DefaultAppConfig())
appconfig.TemplateDirectory = "./templates"
app := cidre.NewApp(appconfig)

root := app.MountPoint("/")
root.Get("page", "page", func(w http.ResponseWriter, r *http.Request) {
  view := cidre.Dict{"key":"value"}
  app.Renderer.Html(w, "template_name", view)
})
~~~

## Sessions

~~~ go
app := cidre.NewApp(cidre.DefaultAppConfig())
sessionConfig := cidre.DefaultSessionConfig()
app.Use(cidre.NewSessionMiddleware(app, sessionConfig, nil))

root := app.MountPoint("/")
root.Get("page", "page", func(w http.ResponseWriter, r *http.Request) {
  ctx := cidre.RequestContext(r)
  ctx.Session.Set("key", "value")
  ctx.Session.Get("key")
  ctx.Session.AddFlash("info", "info message")
})
~~~

## Loading configuration files

app.ini:
~~~
[cidre]
Addr = 127.0.0.1:8080

[session.base]
CookieName = sessionid
Secret = some very secret
~~~
code:
~~~ go
appConfig := cidre.DefaultAppConfig()
sessionConfig := cidre.DefaultSessionConfig()
_, err := cidre.ParseIniFile("app.ini",
	cidre.ConfigMapping{"cidre", appConfig},
	cidre.ConfigMapping{"session.base", sessionConfig},
)
if err != nil {
	panic(err)
}
app := cidre.NewApp(appConfig)
~~~

## Hooks

`cidre.App` and `cidre.ResponseWriter` has some hook points. See [godoc](http://godoc.org/github.com/yuin/cidre) for details.

~~~ go
app := cidre.NewApp(cidre.DefaultAppConfig())
app.Hooks.Add("start_request", func(w http.ResponseWriter, r *http.Request, data interface{}) {
	w.Header().Add("X-Server", "Go")
})

root := app.MountPoint("/")
root.Use(func(w http.ResponseWriter, r *http.Request){

   w.(cidre.ResponseWriter).Hooks().Add("before_write_content", func(w http.ResponseWriter, rnil *http.Request, datanil interface{}) {
      // do some stuff
   })

  cidre.RequestContext(r).MiddlewareChain.DoNext(w,r)

})
~~~
