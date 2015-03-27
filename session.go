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
	sm := &SessionMiddleware{app: app, Config: config}
	if len(sm.Config.Secret) == 0 {
		panic("Session secret must not be empty.")
	}
	DynamicObjectFactory.Register(MemorySessionStore{})
	store, _ := DynamicObjectFactory.New(sm.Config.SessionStore).(SessionStore)
	sm.Store = store
	sm.Store.Init(sm, storeConfig)

	app.Hooks.Add("start_server", func(w http.ResponseWriter, r *http.Request, data interface{}) {
		time.AfterFunc(sm.Config.GcInterval, sm.Gc)
	})

	return sm
}

func (sm *SessionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := RequestContext(r)
	if !ctx.IsDynamicRoute() {
		ctx.MiddlewareChain.DoNext(w, r)
	} else {
		if !strings.HasPrefix(r.URL.Path, sm.Config.CookiePath) {
			return
		}
		func() {
			sm.Store.Lock()
			defer sm.Store.Unlock()
			signedString, _ := r.Cookie(sm.Config.CookieName)
			var session *Session
			if signedString != nil {
				sessionId, err := ValidateSignedString(signedString.Value, sm.Config.Secret)
				if err != nil {
					panic(err)
				}
				session = sm.Store.Load(sessionId)
			} else {
				session = sm.Store.NewSession()
			}
			if session != nil {
				ctx.Session = session
				session.UpdateLastAccessTime()
			}
		}()

		w.(ResponseWriter).Hooks().Add("before_write_header", func(w http.ResponseWriter, rnil *http.Request, statusCode interface{}) {
			if strings.Index(r.URL.Path, sm.Config.CookiePath) != 0 {
				return
			}
			sm.Store.Lock()
			defer sm.Store.Unlock()
            domain := sm.Config.CookieDomain
            if len(domain) == 0 {
              domain = strings.Split(r.Host,":")[0]
            }
			cookie := &http.Cookie{
				Domain: domain,
				Secure: sm.Config.CookieSecure,
				Path:   sm.Config.CookiePath,
                HttpOnly: true,
			}
			if sm.Config.CookieExpires != 0 {
				cookie.Expires = time.Now().Add(sm.Config.CookieExpires)
			}
			session := ctx.Session
			if session == nil {
				return
			}
			if session.Killed {
				cookie.MaxAge = -1
				sm.Store.Delete(session.Id)
			} else {
				sm.Store.Save(session)
			}
			cookie.Name = sm.Config.CookieName
			cookie.Value = SignString(session.Id, sm.Config.Secret)
			http.SetCookie(w, cookie)
		})

		ctx.MiddlewareChain.DoNext(w, r)
	}

}

func (sm *SessionMiddleware) Gc() {
	sm.Store.Lock()
	defer sm.Store.Unlock()
	sm.app.Logger(LogLevelDebug, "Session Gc")
	sm.Store.Gc()
	time.AfterFunc(sm.Config.GcInterval, sm.Gc)
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

func (sess *Session) UpdateLastAccessTime() {
	sess.LastAccessTime = time.Now()
}

func (sess *Session) Kill() {
	sess.Killed = true
}

// Adds a flash message to the session
func (sess *Session) AddFlash(category string, message string) {
	flash := sess.Get(FlashKey).(map[string][]string)
	if _, ok := flash[category]; !ok {
		flash[category] = make([]string, 0, 10)
	}
	flash[category] = append(flash[category], message)
}

// Returns a flash message associated with the given category.
func (sess *Session) Flash(category string) []string {
	flash := sess.Get(FlashKey).(map[string][]string)
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
func (sess *Session) Flashes() map[string][]string {
	flash := sess.Get(FlashKey).(map[string][]string)
	sess.Set(FlashKey, make(map[string][]string))
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

func (ms *MemorySessionStore) Init(middleware *SessionMiddleware, cfg interface{}) {
	ms.middleware = middleware
	ms.store = make(map[string]*Session, 30)
}

func (ms *MemorySessionStore) NewSessionId() string {
	for true {
		now := time.Now().Unix()
		random := strconv.Itoa(rand.Int())
		sessionId := fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("%v%v%v", now, random, ms.middleware.Config.Secret))))
		if !ms.Exists(sessionId) {
			return sessionId
		}
	}
	return ""
}

func (ms *MemorySessionStore) Exists(sessionId string) bool {
	_, ok := ms.store[sessionId]
	return ok
}

func (ms *MemorySessionStore) NewSession() *Session {
	session := NewSession(ms.NewSessionId())
	ms.store[session.Id] = session
	return session
}

func (ms *MemorySessionStore) Save(*Session) { /* Nothing to do */ }

func (ms *MemorySessionStore) Load(sessionId string) *Session {
	session, ok := ms.store[sessionId]
	if ok {
		return session
	}
	return ms.NewSession()
}

func (ms *MemorySessionStore) Delete(sessionId string) {
	delete(ms.store, sessionId)
}

func (ms *MemorySessionStore) Count() int {
	return len(ms.store)
}

func (ms *MemorySessionStore) Gc() {
	delkeys := make([]string, 0, len(ms.store)/10)
	for k, v := range ms.store {
		if (time.Now().Sub(v.LastAccessTime)) > ms.middleware.Config.LifeTime {
			delkeys = append(delkeys, k)
		}
	}
	for _, key := range delkeys {
		ms.Delete(key)
	}
}
