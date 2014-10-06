package cidre

import (
	"crypto/sha1"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SessionConfig is a configuration object for the SessionMiddleware
type SessionConfig struct {
	// default: gossessionid
	CookieName   string
	CookieDomain string
	// default: false
	CookieSecure  bool
	CookiePath    string
	CookieExpires time.Duration
	// A term used to authenticate the cookie value using HMAC
	Secret string
	// default: "cidre.MemorySessionStore"
	SessionStore string
	// default: 30m
	GcInterval time.Duration
	// default: 30m
	LifeTime time.Duration
}

// Returns a SessionConfig object that has default values set.
// If an 'init' function object argument is not nil, this function
// will call the function with the SessionConfig object.
func DefaultSessionConfig(init ...func(*SessionConfig)) *SessionConfig {
	self := &SessionConfig{
		CookieName:    "gosessionid",
		CookieDomain:  "",
		CookieSecure:  false,
		CookiePath:    "",
		CookieExpires: 0,
		Secret:        "",
		SessionStore:  "cidre.MemorySessionStore",
		GcInterval:    time.Minute * 30,
		LifeTime:      time.Minute * 30,
	}
	if len(init) > 0 {
		init[0](self)
	}
	return self
}

// Middleware for session management.
type SessionMiddleware struct {
	app    *App
	Config *SessionConfig
	Store  SessionStore
}

// Returns a new SessionMiddleware object.
func NewSessionMiddleware(app *App, config *SessionConfig, storeConfig interface{}) *SessionMiddleware {
	self := &SessionMiddleware{app: app, Config: config}
	if len(self.Config.Secret) == 0 {
		panic("Session secret must not be empty.")
	}
	DynamicObjectFactory.Register(MemorySessionStore{})
	store, _ := DynamicObjectFactory.New(self.Config.SessionStore).(SessionStore)
	self.Store = store
	self.Store.Init(self, storeConfig)

	app.Hooks.Add("start_server", func(w http.ResponseWriter, r *http.Request, data interface{}) {
		time.AfterFunc(self.Config.GcInterval, self.Gc)
	})

	return self
}

func (self *SessionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := RequestContext(r)
	if !ctx.IsDynamicRoute() {
		ctx.MiddlewareChain.DoNext(w, r)
	} else {
		if !strings.HasPrefix(r.URL.Path, self.Config.CookiePath) {
			return
		}
		func() {
			self.Store.Lock()
			defer self.Store.Unlock()
			signedString, _ := r.Cookie(self.Config.CookieName)
			var session *Session
			if signedString != nil {
				sessionId, err := ValidateSignedString(signedString.Value, self.Config.Secret)
				if err != nil {
					panic(err)
				}
				session = self.Store.Load(sessionId)
			} else {
				session = self.Store.NewSession()
			}
			if session != nil {
				ctx.Session = session
				session.UpdateLastAccessTime()
			}
		}()

		w.(ResponseWriter).Hooks().Add("after_write_header", func(w http.ResponseWriter, rnil *http.Request, datanil interface{}) {
			if strings.Index(r.URL.Path, self.Config.CookiePath) != 0 {
				return
			}
			self.Store.Lock()
			defer self.Store.Unlock()
			cookie := &http.Cookie{
				Domain: self.Config.CookieDomain,
				Secure: self.Config.CookieSecure,
				Path:   self.Config.CookiePath,
			}
			if self.Config.CookieExpires != 0 {
				cookie.Expires = time.Now().Add(self.Config.CookieExpires)
			}
			session := ctx.Session
			if session == nil {
				return
			}
			if session.Killed {
				cookie.MaxAge = -1
				self.Store.Delete(session.Id)
			} else {
				self.Store.Save(session)
			}
			cookie.Name = self.Config.CookieName
			cookie.Value = SignString(session.Id, self.Config.Secret)
			http.SetCookie(w, cookie)
		})

		ctx.MiddlewareChain.DoNext(w, r)
	}

}

func (self *SessionMiddleware) Gc() {
	self.Store.Lock()
	defer self.Store.Unlock()
	self.app.Logger(LogLevelDebug, "Session Gc")
	self.Store.Gc()
	time.AfterFunc(self.Config.GcInterval, self.Gc)
}

// Session value container.
type Session struct {
	Dict
	Killed         bool
	Id             string
	LastAccessTime time.Time
}

const FlashKey = "_flash"

func NewSession(id string) *Session {
	self := &Session{
		Dict:   NewDict(),
		Killed: false, Id: id,
		LastAccessTime: time.Now()}
	self.Set(FlashKey, make(map[string][]string))
	return self
}

func (self *Session) UpdateLastAccessTime() {
	self.LastAccessTime = time.Now()
}

func (self *Session) Kill() {
	self.Killed = true
}

// Adds a flash message to the session
func (self *Session) AddFlash(category string, message string) {
	flash := self.Get(FlashKey).(map[string][]string)
	if _, ok := flash[category]; !ok {
		flash[category] = make([]string, 0, 10)
	}
	flash[category] = append(flash[category], message)
}

// Returns a flash message associated with the given category.
func (self *Session) Flash(category string) []string {
	flash := self.Get(FlashKey).(map[string][]string)
	v, ok := flash[category]
	delete(flash, category)
	if !ok {
		return make([]string, 0, 10)
	}
	return v
}

// Returns a list of flash messages from the session.
//
//     session.AddFlash("info", "info message1")
//     session.AddFlash("info", "info message2")
//     session.AddFlash("error", "error message")
//     messages := session.Flashes()
//     // -> {"info":["info message1", "info message2"], "error":["error message"]}
func (self *Session) Flashes() map[string][]string {
	flash := self.Get(FlashKey).(map[string][]string)
	self.Set(FlashKey, make(map[string][]string))
	return flash
}

// SessionStore is an interface for custom session stores.
// See the MemorySessionStore for examples.
type SessionStore interface {
	Lock()
	Unlock()
	Init(*SessionMiddleware, interface{})
	Exists(string) bool
	NewSession() *Session
	Save(*Session)
	Load(string) *Session
	Delete(string)
	Gc()
	Count() int
}

type MemorySessionStore struct {
	sync.Mutex
	middleware *SessionMiddleware
	store      map[string]*Session
}

func (self *MemorySessionStore) Init(middleware *SessionMiddleware, cfg interface{}) {
	self.middleware = middleware
	self.store = make(map[string]*Session, 30)
}

func (self *MemorySessionStore) NewSessionId() string {
	for true {
		now := time.Now().Unix()
		random := strconv.Itoa(rand.Int())
		sessionId := fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%v%v%v", now, random, self.middleware.Config.Secret))))
		if !self.Exists(sessionId) {
			return sessionId
		}
	}
	return ""
}

func (self *MemorySessionStore) Exists(sessionId string) bool {
	_, ok := self.store[sessionId]
	return ok
}

func (self *MemorySessionStore) NewSession() *Session {
	session := NewSession(self.NewSessionId())
	self.store[session.Id] = session
	return session
}

func (self *MemorySessionStore) Save(*Session) { /* Nothing to do */ }

func (self *MemorySessionStore) Load(sessionId string) *Session {
	session, ok := self.store[sessionId]
	if ok {
		return session
	}
	return self.NewSession()
}

func (self *MemorySessionStore) Delete(sessionId string) {
	delete(self.store, sessionId)
}

func (self *MemorySessionStore) Count() int {
	return len(self.store)
}

func (self *MemorySessionStore) Gc() {
	delkeys := make([]string, 0, len(self.store)/10)
	for k, v := range self.store {
		if (time.Now().Sub(v.LastAccessTime)) > self.middleware.Config.LifeTime {
			delkeys = append(delkeys, k)
		}
	}
	for _, key := range delkeys {
		self.Delete(key)
	}
}
