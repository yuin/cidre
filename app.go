package cidre

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"text/template"
	"time"
)

/* Context {{{ */

// Context is a per-request context object. It allows us to share variables between middlewares.
type Context struct {
	Dict
	App             *App
	Session         *Session
	Id              string
	Route           *Route
	PathParams      *url.Values
	StartedAt       time.Time
	ResponseTime    time.Duration
	MiddlewareChain *MiddlewareChain
}

type contextBody struct {
	io.ReadCloser
	Context *Context
}

// Returns a new Context object.
func NewContext(app *App, id string, r *http.Request) *Context {
	tmp := r.Body
	context := &Context{
		Dict:       NewDict(),
		App:        app,
		Id:         id,
		PathParams: &url.Values{},
	}
	r.Body = &contextBody{tmp, context}
	return context
}

// Returns true if the matched route is dynamic, false if there is no matched
// routes or the matched route is for static files.
func (ctx *Context) IsDynamicRoute() bool {
	return ctx.Route != nil && !ctx.Route.IsStatic
}

// Returns a contenxt object associated with the given request.
func RequestContext(r *http.Request) *Context {
	return r.Body.(*contextBody).Context
}

/* }}} */

/* Hooks {{{ */

// Hooks is a container of Hook objects.
type Hooks map[string][]Hook

// Hook is a mechanism for customization of cidre.
// Hook is a function, to be called on some well-defined occasion.
type Hook func(http.ResponseWriter, *http.Request, interface{})

// HookDirection represents execution order of Hooks.
type HookDirection int

const (
	// from front to back
	HookDirectionNormal HookDirection = iota
	// from back to front
	HookDirectionReverse
)

// Executes hooks associated with the given name.
func (hooks Hooks) Run(name string, direction HookDirection, w http.ResponseWriter, r *http.Request, data interface{}) {
	if direction == HookDirectionNormal {
		for _, hook := range hooks[name] {
			hook(w, r, data)
		}
	} else {
		s := hooks[name]
		for i := len(s) - 1; i >= 0; i-- {
			s[i](w, r, data)
		}
	}
}

// Registers a hook to be executed at the given hook point.
func (hooks Hooks) Add(name string, hook Hook) {
	_, ok := hooks[name]
	if !ok {
		hooks[name] = make([]Hook, 0, 10)
	}
	hooks[name] = append(hooks[name], hook)
}

/* }}} */

/* ResponseWriter {{{ */

// ResponseWriter is a wrapper around http.ResponseWriter that provides extra methods about the response.
//
// Hook points:
//     - before_write_header(self, nil, status int)
//     - after_write_header(self, nil, status int)
//     - before_write_content(self, nil, content []byte)
type ResponseWriter interface {
	http.ResponseWriter
	SetHeader(int)
	ContentLength() int
	Status() int
	Hooks() Hooks
}

type responseWriter struct {
	http.ResponseWriter
	status        int
	contentLength int
	hooks         Hooks
}

// Returns a new ResponseWriter object wrap around the given http.ResponseWriter object.
func NewResponseWriter(w http.ResponseWriter) ResponseWriter {
	self := &responseWriter{w, 0, 0, make(Hooks)}
	return self
}

func (w *responseWriter) Hooks() Hooks {
	return w.hooks
}

func (w *responseWriter) SetHeader(status int) {
	w.status = status
}

func (w *responseWriter) WriteHeader(status int) {
	w.Hooks().Run("before_write_header", HookDirectionReverse, w, nil, status)
	w.status = status
	w.ResponseWriter.WriteHeader(status)
	w.Hooks().Run("after_write_header", HookDirectionReverse, w, nil, status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.ContentLength() == 0 {
		if w.status == 0 {
			w.status = 200
		}
		w.WriteHeader(w.status)
		w.Hooks().Run("before_write_content", HookDirectionReverse, w, nil, b)
	}

	i, err := w.ResponseWriter.Write(b)
	if err == nil {
		w.contentLength += len(b)
	}
	return i, err
}

func (w *responseWriter) ContentLength() int {
	return w.contentLength
}

func (w *responseWriter) Status() int {
	return w.status
}

/* }}} */

/* Middleware {{{ */

// Middleware is an ailias for the http.Handler interface.
// In ServeHTTP, you should yield to the next middleware in the chain.
type Middleware http.Handler

// MiddlewareChain represents an invocation chain of a middleware.
// Middlewares use the MiddlewareChain to invoke the next middleware in the chain,
// or if the calling middleware is the last middleware in the chain,
// to invoke the handler at the end of the chain.
type MiddlewareChain struct {
	middlewares []Middleware
	sp          int
}

// Returns a new MiddlewareChain object.
func NewMiddlewareChain(middlewares []Middleware) *MiddlewareChain {
	return &MiddlewareChain{middlewares, -1}
}

// Returns a copy of the MiddlewareChain object.
func (mc *MiddlewareChain) Copy() *MiddlewareChain {
	return NewMiddlewareChain(mc.middlewares)
}

// Causes the next middleware in the chain to be invoked, or if the calling middleware is
// the last middleware in the chain, causes the handler at the end of the chain to be invoked.
func (mc *MiddlewareChain) DoNext(w http.ResponseWriter, r *http.Request) {
	mc.sp += 1
	mc.middlewares[mc.sp].ServeHTTP(w, r)
}

func MiddlewareOf(arg interface{}) Middleware {
	switch arg.(type) {
	case http.Handler:
		return arg.(Middleware)
	default:
		return Middleware(http.HandlerFunc(arg.(func(http.ResponseWriter, *http.Request))))
	}
}

func MiddlewaresOf(args ...interface{}) []Middleware {
	result := make([]Middleware, 0, len(args))
	for _, arg := range args {
		result = append(result, MiddlewareOf(arg))
	}
	return result
}

/* }}} */

/* Logger {{{ */

type Logger func(LogLevel, string)

type LogLevel int

const (
	LogLevelUnknown LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelCrit
)

var logLevelStrings = map[LogLevel]string{
	LogLevelDebug: "DEBUG", LogLevelInfo: "INFO",
	LogLevelWarn: "WARN", LogLevelError: "ERROR", LogLevelCrit: "CRIT",
}

func (ll LogLevel) String() string {
	if v, ok := logLevelStrings[ll]; !ok {
		return "UNKNOWN"
	} else {
		return v
	}
}

func DefaultLogger(level LogLevel, message string) {
	fmt.Fprintln(os.Stdout, BuildString(256, time.Now().Format(time.RFC3339), "\t", level.String(), "\t", message))
}

/* }}} */

/* Route {{{ */

// Route represents a Route in cidre. Route implements the Middleware interface.
type Route struct {
	Name            string
	PathParamNames  []string
	Method          string
	Pattern         *regexp.Regexp
	PatternString   string
	IsStatic        bool
	MiddlewareChain *MiddlewareChain
	Meta            Dict
}

var NopMiddleware = Middleware(MiddlewareOf(func(w http.ResponseWriter, r *http.Request) {}))

func NewRoute(n, p, m string, s bool, handler http.Handler, middlewares ...Middleware) *Route {
	self := &Route{
		Name:          n,
		Pattern:       regexp.MustCompile("^" + p + "$"),
		PatternString: p,
		Method:        m,
		IsStatic:      s,
		Meta:          make(Dict),
	}
	reg := regexp.MustCompile("\\?P<([^<]+)>")
	for _, lst := range reg.FindAllStringSubmatch(p, -1) {
		self.PathParamNames = append(self.PathParamNames, lst[1])
	}
	mds := make([]Middleware, 0, 20)
	mds = append(mds, middlewares...)
	mds = append(mds, Middleware(handler), NopMiddleware)
	self.MiddlewareChain = NewMiddlewareChain(mds)
	return self
}

func (route *Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := RequestContext(r)
	ctx.MiddlewareChain = route.MiddlewareChain.Copy()
	ctx.MiddlewareChain.DoNext(w, r)
}

/* }}} */

/* MountPoint {{{ */

// MountPoint represents a group of routes that has same URL prefix and
// a set of middlewares.
type MountPoint struct {
	App         *App
	Path        string
	Middlewares []Middleware
}

// Adds a middleware to the end of the middleware chain.
func (mt *MountPoint) Use(middlewares ...interface{}) {
	mt.Middlewares = append(mt.Middlewares, MiddlewaresOf(middlewares...)...)
}

// Registers a http.HandlerFunc and middlewares with the given path pattern and method.
func (mt *MountPoint) Route(n, p, m string, s bool, h http.HandlerFunc, middlewares ...interface{}) *Route {
	mds := make([]Middleware, 0, 10)
	mds = append(mds, mt.Middlewares...)
	mds = append(mds, MiddlewaresOf(middlewares...)...)
	route := NewRoute(n, mt.Path+p, m, s, http.HandlerFunc(h), mds...)
	mt.App.Routes[n] = route
	return route
}

// Shortcut for Route(name, pattern, "GET", false, handler, ...Middleware)
func (mt *MountPoint) Get(n, p string, h http.HandlerFunc, middlewares ...interface{}) *Route {
	return mt.Route(n, p, "GET", false, h, middlewares...)
}

// Shortcut for Route(name, pattern, "POST", false, handler, ...Middleware)
func (mt *MountPoint) Post(n, p string, h http.HandlerFunc, middlewares ...interface{}) *Route {
	return mt.Route(n, p, "POST", false, h, middlewares...)
}

// Shortcut for Route(name, pattern, "Put", false, handler, ...Middleware)
func (mt *MountPoint) Put(n, p string, h http.HandlerFunc, middlewares ...interface{}) *Route {
	return mt.Route(n, p, "PUT", false, h, middlewares...)
}

// Shortcut for Route(name, pattern, "DELETE", false, handler, ...Middleware)
func (mt *MountPoint) Delete(n, p string, h http.HandlerFunc, middlewares ...interface{}) *Route {
	return mt.Route(n, p, "DELETE", false, h, middlewares...)
}

// Registers a handler that serves static files.
func (mt *MountPoint) Static(n, p, local string, middlewares ...interface{}) *Route {
	path := strings.Trim(p, "/")
	server := http.StripPrefix(mt.Path+path, http.FileServer(http.Dir(local)))
	rt := mt.Route(n, path+"/(?P<path>.*)", "GET", true, server.ServeHTTP, middlewares...)
	rt.Meta.Set("local", local)
	return rt
}

/* }}} */

/* App {{{ */

// AppConfig is a configuration object for the App struct.
type AppConfig struct {
	// default : false
	Debug bool
	// Server address, default:"127.0.0.1:8080"
	Addr string
	// default: ""
	TemplateDirectory string
	// default: true, if this value is true, cidre will treat a "_method" parameter as a HTTP method name.
	AllowHttpMethodOverwrite bool
	// cidre uses text/template to format access logs.
	// default: "{{.c.Id}} {{.req.RemoteAddr}} {{.req.Method}} {{.req.RequestURI}} {{.req.Proto}} {{.res.Status}} {{.res.ContentLength}} {{.c.ResponseTime}}"
	AccessLogFormat string
	// default: 180s
	ReadTimeout time.Duration
	// default: 180s
	WriteTimeout time.Duration
	// default: 8192
	MaxHeaderBytes int
	// default: false
	KeepAlive bool
	// calls runtime.GOMAXPROCS(runtime.NumCPU()) when server starts if AutoMaxProcs is true.
	// default: true
	AutoMaxProcs bool
}

// Returns a new AppConfig object that has default values set.
// If an 'init' function object argument is not nil, this function
// will call the function with the AppConfig object.
func DefaultAppConfig(init ...func(*AppConfig)) *AppConfig {
	self := &AppConfig{
		Debug:                    false,
		Addr:                     "127.0.0.1:8080",
		TemplateDirectory:        "",
		AllowHttpMethodOverwrite: true,
		AccessLogFormat:          "{{.c.Id}} {{.req.RemoteAddr}} {{.req.Method}} {{.req.RequestURI}} {{.req.Proto}} {{.res.Status}} {{.res.ContentLength}} {{.c.ResponseTime}}",
		ReadTimeout:              time.Second * 180,
		WriteTimeout:             time.Second * 180,
		MaxHeaderBytes:           8192,
		KeepAlive:                false,
		AutoMaxProcs:             true,
	}
	if len(init) > 0 {
		init[0](self)
	}
	return self
}

// App represents a web application.
// Hooks:
//   - setup(nil, nil, self)
//   - start_server(nil, nil, self)
//   - start_request(http.ResponseWriter, *http.Request, nil)
//   - start_action(http.ResponseWriter, *http.Request, nil)
//   - end_action(http.ResponseWriter, *http.Request, nil)
//   - end_request(http.ResponseWriter, *http.Request, nil)
type App struct {
	Config       *AppConfig
	Routes       map[string]*Route
	Middlewares  []Middleware
	Logger       Logger
	AccessLogger Logger
	// handlers to be called if errors was occurred during a request.
	OnPanic func(http.ResponseWriter, *http.Request, interface{})
	// handlers to be called if no suitable routes found.
	OnNotFound        func(http.ResponseWriter, *http.Request)
	Renderer          Renderer
	Hooks             Hooks
	contextIdSeq      uint32
	accessLogTemplate *template.Template
}

// Returns a new App object.
func NewApp(config *AppConfig) *App {
	self := &App{
		Config:       config,
		Routes:       make(map[string]*Route),
		Middlewares:  make([]Middleware, 0, 5),
		Logger:       DefaultLogger,
		AccessLogger: DefaultLogger,
		Renderer:     nil,
		contextIdSeq: 0,
		Hooks:        make(Hooks),
	}
	self.OnPanic = self.DefaultOnPanic
	self.OnNotFound = self.DefaultOnNotFound
	return self
}

func (app *App) newContextId() string {
	now := time.Now()
	return fmt.Sprintf("%04d%02d%02d%02d%02d%010d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), atomic.AddUint32(&(app.contextIdSeq), 1))
}

func (app *App) DefaultOnPanic(w http.ResponseWriter, r *http.Request, rcv interface{}) {
	if app.Config.Debug {
		http.Error(w, fmt.Sprintf("%v:\n\n%s", rcv, debug.Stack()), http.StatusInternalServerError)
	} else {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (app *App) DefaultOnNotFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// Builds an url for the given named route with path parameters.
func (app *App) BuildUrl(n string, args ...string) string {
	route, ok := app.Routes[n]
	if !ok {
		panic(fmt.Sprintf("Route '%v' not defined.", n))
	}
	reg := regexp.MustCompile(`\(\?P<([^<]+)>[^\)]+\)`)
	counter := -1
	return reg.ReplaceAllStringFunc(route.PatternString, func(m string) string {
		counter += 1
		return args[counter]
	})
}

// Adds a middleware to the end of the middleware chain.
func (app *App) Use(middlewares ...interface{}) {
	app.Middlewares = append(app.Middlewares, MiddlewaresOf(middlewares...)...)
}

// Returns a new MountPoint object associated the given path.
func (app *App) MountPoint(path string) *MountPoint {
	mp := &MountPoint{app, strings.TrimRight(path, "/") + "/", make([]Middleware, 0, len(app.Middlewares)+5)}
	mp.Middlewares = append(mp.Middlewares, app.Middlewares...)
	return mp
}

func (app *App) cleanup(w http.ResponseWriter, r *http.Request) {
	if rcv := recover(); rcv != nil {
		app.OnPanic(w, r, rcv)
	}
	ctx := RequestContext(r)
	ctx.ResponseTime = time.Now().Sub(ctx.StartedAt)
	app.Hooks.Run("end_request", HookDirectionReverse, w, r, nil)
}

func (app *App) ServeHTTP(ww http.ResponseWriter, r *http.Request) {
	w := NewResponseWriter(ww)
	ctx := NewContext(app, app.newContextId(), r)
	ctx.StartedAt = time.Now()

	defer app.cleanup(w, r)

	app.Hooks.Run("start_request", HookDirectionNormal, w, r, nil)

	path := r.URL.Path
	method := r.Method
	if app.Config.AllowHttpMethodOverwrite {
		if overwrittenMethod := r.PostFormValue("_method"); len(overwrittenMethod) > 0 {
			method = overwrittenMethod
		}
	}
	for _, route := range app.Routes {
		if strings.ToUpper(method) != strings.ToUpper(route.Method) {
			continue
		}

		submatches := route.Pattern.FindStringSubmatch(path)
		if len(submatches) > 0 {
			for i, pathParamName := range route.PathParamNames {
				ctx.PathParams.Add(pathParamName, submatches[i+1])
			}
			ctx.Route = route
		}
		if ctx.Route != nil {
			break
		}
	}

	if ctx.Route == nil {
		app.OnNotFound(w, r)
		return
	}

	app.Hooks.Run("start_action", HookDirectionNormal, w, r, nil)
	ctx.Route.ServeHTTP(w, r)
	app.Hooks.Run("end_action", HookDirectionReverse, w, r, nil)
}

func (app *App) writeAccessLog(w http.ResponseWriter, r *http.Request, d interface{}) {
	data := map[string]interface{}{
		"c":   RequestContext(r),
		"res": w,
		"req": r,
	}
	var b bytes.Buffer
	app.accessLogTemplate.Execute(&b, data)
	s := b.String()
	app.AccessLogger(LogLevelInfo, s)
}

//
func (app *App) Setup() {
	if app.Renderer == nil {
		cfg := DefaultHtmlTemplateRendererConfig()
		cfg.TemplateDirectory = app.Config.TemplateDirectory
		app.Renderer = NewHtmlTemplateRenderer(cfg)
	}
	app.Hooks.Add("end_request", app.writeAccessLog)
	app.Hooks.Run("setup", HookDirectionNormal, nil, nil, app)
	if app.Config.AutoMaxProcs {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	app.Renderer.Compile()
	tmpl, err := template.New("cidre.acccesslog").Parse(app.Config.AccessLogFormat)
	if err != nil {
		panic(err)
	}
	app.accessLogTemplate = tmpl
}

// Returns a new http.Server object.
func (app *App) Server() *http.Server {
	server := &http.Server{
		Addr:           app.Config.Addr,
		Handler:        app,
		ReadTimeout:    app.Config.ReadTimeout,
		WriteTimeout:   app.Config.WriteTimeout,
		MaxHeaderBytes: app.Config.MaxHeaderBytes,
	}
	server.SetKeepAlivesEnabled(app.Config.KeepAlive)
	return server
}

// Run the http.Server. If _server is not passed, App.Server() will be used as a http.Server object.
func (app *App) Run(_server ...*http.Server) {
	if app.accessLogTemplate == nil {
		app.Setup()
	}
	var server *http.Server
	if len(_server) > 0 {
		server = _server[0]
	} else {
		server = app.Server()
	}
	app.Hooks.Run("start_server", HookDirectionNormal, nil, nil, app)
	app.Logger(LogLevelInfo, fmt.Sprintf("Server started: addr=%v", app.Config.Addr))
	server.ListenAndServe()
}

/* }}} */
