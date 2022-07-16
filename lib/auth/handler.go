package auth

import (
	"fmt"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/openidConnect"
	"github.com/unrolled/render"
)

const (
	authKey   = "janus_auth_user"
	returnKey = "janus_auth_return"
)

// Init sets up the SSO library. Must be called exactly once, before any
// requests are served.
func Init(p *openidConnect.Provider) {
	goth.ClearProviders()
	goth.UseProviders(p)

	gothic.GetProviderName = func(*http.Request) (string, error) {
		return "openid-connect", nil
	}
}

// RegisterRoutes configures the Router to handle authentication related requests.
func RegisterRoutes(router *httprouter.Router) {
	// Gitlab doesn't like trailing slashes in the callback URL.
	router.GET("/auth/callback", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		guser, err := gothic.CompleteUserAuth(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		postAuth(w, r, guser)
	})

	router.GET("/auth/logout/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		gothic.Logout(w, r)
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})

	router.GET("/auth/login/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		// try to get the user without re-authenticating
		if guser, err := gothic.CompleteUserAuth(w, r); err == nil {
			postAuth(w, r, guser)
			return
		}
		gothic.BeginAuthHandler(w, r)
	})
}

func Get(r *http.Request) (*User, error) {
	s, err := gothic.GetFromSession(authKey, r)
	if err != nil {
		return nil, fmt.Errorf("getting from session: %v", err)
	}
	u, err := parseUser(s)
	if err != nil {
		return nil, fmt.Errorf("parsing user: %v", err)
	}
	return u, nil
}

func MustBeAuthed(delegate httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if _, err := Get(r); err != nil {
			_ = gothic.StoreInSession(returnKey, r.URL.Path, r, w)
			http.Redirect(w, r, "/auth/login/", http.StatusTemporaryRedirect)
			return
		}
		delegate(w, r, p)
	}
}

func MustHaveGroup(rend *render.Render, group string, delegate httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		u, err := Get(r)
		if err != nil {
			_ = gothic.StoreInSession(returnKey, r.URL.Path, r, w)
			http.Redirect(w, r, "/auth/login/", http.StatusTemporaryRedirect)
			return
		}
		for _, g := range u.Groups {
			if g == group {
				delegate(w, r, p)
				return
			}
		}
		rend.HTML(w, http.StatusUnauthorized, "error", "You do not have access to this resource.")
	}
}

func postAuth(w http.ResponseWriter, r *http.Request, guser goth.User) {
	user, err := fromGothUser(guser)
	if err != nil {
		log.Printf("[ERROR] adding user to session: fromGothUser: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := gothic.StoreInSession(authKey, user.String(), r, w); err != nil {
		log.Printf("[ERROR] adding user to session: StoreInSession: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Somehow this isn't working... fix it!
	url, err := gothic.GetFromSession(returnKey, r)
	if err != nil {
		log.Printf("[WARNING] getting return url from session: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	gothic.StoreInSession(returnKey, "", r, w)
	if url == "" {
		url = "/"
	}
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}
