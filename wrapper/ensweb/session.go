package ensweb

import (
	"crypto/sha256"
	"fmt"

	"github.com/gorilla/sessions"
)

type SessionStore struct {
	store  *sessions.CookieStore
	name   string
	option sessions.Options
}

func (s *Server) CreateSessionStore(name string, secret string, option sessions.Options) {
	h := sha256.New()
	h.Write([]byte(secret))
	key := h.Sum(nil)
	store := sessions.NewCookieStore(key)
	store.Options = &option
	ss := SessionStore{
		store:  store,
		name:   name,
		option: option,
	}
	s.ss[name] = &ss
}

func (s *Server) SetSessionCookies(req *Request, sessionName string, key interface{}, value interface{}) error {
	if s.ss != nil && s.ss[sessionName] != nil {
		ss := s.ss[sessionName]

		sess, err := ss.store.Get(req.r, sessionName)
		if err != nil {
			return err
		}
		sess.Values[key] = value
		return sess.Save(req.r, req.w)
	}
	return fmt.Errorf("invalid session")
}

func (s *Server) GetSessionCookies(req *Request, sessionName string, key interface{}) interface{} {
	if s.ss != nil && s.ss[sessionName] != nil {
		ss := s.ss[sessionName]

		sess, err := ss.store.Get(req.r, sessionName)
		if err != nil {
			return nil
		}
		return sess.Values[key]
	}
	return nil
}

func (s *Server) EmptySessionCookies(req *Request, sessionName string) error {
	if s.ss != nil && s.ss[sessionName] != nil {
		ss := s.ss[sessionName]
		sess, err := ss.store.Get(req.r, sessionName)
		if err != nil {
			return err
		}
		// Clear out all stored values in the cookie
		for k := range sess.Values {
			delete(sess.Values, k)
		}
		return sess.Save(req.r, req.w)
	}
	return fmt.Errorf("invalid session")
}
