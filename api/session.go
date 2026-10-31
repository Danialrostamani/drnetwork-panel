package api

import (
	"encoding/gob"
	"net"
	"net/http"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUser = "LOGIN_USER"
	// loginEpoch is the account's session version the session was issued
	// with; see model.User.SessionEpoch.
	loginEpoch = "LOGIN_EPOCH"
	// checkedLoginKey keeps the checked user on the request, so the database
	// is asked once per request rather than by every GetLoginUser.
	checkedLoginKey = "drnetwork.loginUser"
)

var sessionUsers service.UserService

func init() {
	gob.Register(model.User{})
}

// BaseSessionOptions are the cookie attributes that do not depend on the
// request, so the session store and every login share one definition.
//
// HttpOnly keeps the session out of reach of script: the panel renders
// operator-supplied strings in several places, and without it any one of them
// turning into an XSS hands over the session outright. The frontend router
// used to read this cookie to decide whether it was logged in, which is why
// this could not be set before; it now tracks that itself.
//
// SameSite=Strict is safe in both HTTP and HTTPS mode -- it has nothing to do
// with TLS -- and stops a cross-site request from carrying the session at all.
func BaseSessionOptions(maxAgeMinutes int) sessions.Options {
	o := sessions.Options{
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	if maxAgeMinutes > 0 {
		o.MaxAge = maxAgeMinutes * 60
	}
	return o
}

func sessionOptions(c *gin.Context, maxAgeMinutes int) sessions.Options {
	o := BaseSessionOptions(maxAgeMinutes)
	o.Secure = requestIsHTTPS(c)
	return o
}

// requestIsHTTPS reports whether the browser reached the panel over TLS.
//
// It must be derived per request, never hardcoded: a browser will not send a
// Secure cookie over plain HTTP, so a fixed true breaks login on every
// HTTP-only install, and a fixed false gives up the protection on HTTPS ones.
//
// Behind a reverse proxy the panel itself speaks plain HTTP, so the forwarded
// scheme is the only evidence there. It is believed only from a loopback or
// private peer -- otherwise anyone able to reach an HTTP-only panel directly
// could set the header and make the browser refuse to send the cookie back,
// locking the operator out of their own panel.
func requestIsHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if !strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return false
	}
	ip := net.ParseIP(c.RemoteIP())
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// SetLoginUser starts a session for userName at the account's current session
// version. It is also how a request whose own change ended the account's
// sessions (new credentials, two-factor on or off) keeps its own.
func SetLoginUser(c *gin.Context, userName string, maxAge int) error {
	epoch, _, err := sessionUsers.SessionEpoch(userName)
	if err != nil {
		return err
	}
	s := sessions.Default(c)
	s.Set(loginUser, userName)
	s.Set(loginEpoch, epoch)
	s.Options(sessionOptions(c, maxAge))
	if err := s.Save(); err != nil {
		return err
	}
	c.Set(checkedLoginKey, userName)
	return nil
}

func SetMaxAge(c *gin.Context) error {
	s := sessions.Default(c)
	s.Options(sessionOptions(c, 0))
	return s.Save()
}

// GetLoginUser is the user the request's session belongs to, or "" when it
// has none that still holds. The cookie is signed, but signed is not enough:
// it names a user and nothing on the server can take it back, so a cookie
// copied before the password changed kept working until it expired. A session
// now also holds while its version is the account's current one and the
// account exists.
func GetLoginUser(c *gin.Context) string {
	if v, ok := c.Get(checkedLoginKey); ok {
		name, _ := v.(string)
		return name
	}
	name := sessionLoginUser(c)
	c.Set(checkedLoginKey, name)
	return name
}

func sessionLoginUser(c *gin.Context) string {
	s := sessions.Default(c)
	name, ok := s.Get(loginUser).(string)
	if !ok || name == "" {
		return ""
	}
	var issued int64
	switch v := s.Get(loginEpoch).(type) {
	case int64:
		issued = v
	case int:
		issued = int64(v)
	}
	current, found, err := sessionUsers.SessionEpoch(name)
	if err != nil {
		// A database that cannot be read right now says nothing about the
		// session. Failing closed here would sign the operator out on every
		// busy moment of the database.
		logger.Warning("session check: ", err)
		return name
	}
	if !found || current != issued {
		return ""
	}
	return name
}

func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != ""
}

func ClearSession(c *gin.Context) {
	s := sessions.Default(c)
	s.Clear()
	// Same attributes as the cookie being replaced. A browser matches the
	// deletion against Path and Secure, so an expiry written with different
	// attributes leaves the original cookie in place.
	o := sessionOptions(c, 0)
	o.MaxAge = -1
	s.Options(o)
	s.Save()
	c.Set(checkedLoginKey, "")
}
