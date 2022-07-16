package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/mailgun/mailgun-go/v4"
	mattermost "github.com/mattermost/mattermost-server/v6/model"
	"github.com/unrolled/render"
	"github.com/xanzy/go-gitlab"
	janus "gitlab.operationuplift.work/operations/development/janus/lib"
	"gitlab.operationuplift.work/operations/development/janus/lib/auth"
	"gitlab.operationuplift.work/operations/development/janus/lib/provisioner"
	"gitlab.operationuplift.work/operations/development/janus/lib/useradmin"
)

var (
	listenAddr      = env("JANUS_LISTEN_ADDR", ":3149")
	configFile      = env("JANUS_CONFIG_FILE", "./config/janus-config.toml")
	mattermostURL   = env("JANUS_MATTERMOST_URL", "http://127.0.0.1:8065/")
	mattermostToken = env("JANUS_MATTERMOST_TOKEN", "" /*DO NOT PUT IT HERE !!*/)
	gitlabURL       = env("JANUS_GITLAB_URL", "http://127.0.0.1:8929/")
	gitlabToken     = env("JANUS_GITLAB_TOKEN", "" /*DO NOT PUT IT HERE!!*/)
	emailFromAddr   = env("JANUS_EMAIL_FROM_ADDR", "")
	openIDKey       = env("JANUS_OPENID_KEY", "")
	openIDSecret    = env("JANUS_OPENID_SECRET", "")
	openIDScopes    = env("JANUS_OPENID_SCOPES", "email")
	openIDCallback  = env("JANUS_OPENID_CALLBACK", "")
	openIDDiscovery = env("JANUS_OPENID_DISCOVERY", "")

	// MG_DOMAIN and MG_API_KEY also required for Mailgun.
)

func main() {
	oidCfg, err := auth.Provider(&auth.OpenIDConfig{
		Key:          openIDKey,
		Secret:       openIDSecret,
		Scopes:       strings.Split(openIDScopes, ","),
		CallbackURL:  openIDCallback,
		DiscoveryURL: openIDDiscovery,
	})
	if err != nil {
		log.Fatalf("Creating OpenID provider: %v", err)
	}
	auth.Init(oidCfg)

	mmc := mattermost.NewAPIv4Client(mattermostURL)
	mmc.SetToken(mattermostToken)
	if str, _, err := mmc.GetPing(); err != nil {
		log.Fatal("Could not ping Mattermost server:", err)
	} else {
		log.Printf("Pinged Mattermost server: %s", str)
	}

	glc, err := gitlab.NewClient(gitlabToken, gitlab.WithBaseURL(gitlabURL))
	if err != nil {
		log.Fatalf("Could not create Gitlab client: %v", err)
	}
	ver, _, err := glc.Version.GetVersion()
	if err != nil {
		log.Fatalf("Could not get Gitlab version: %v", err)
	} else {
		log.Printf("Connected to Gitlab version %v.", ver)
	}

	mgc, err := mailgun.NewMailgunFromEnv()
	if err != nil {
		log.Fatalf("Creating Mailgun client: %v", err)
	}

	rend := render.New(render.Options{
		Layout:        "layout",
		IsDevelopment: true,
		Funcs: []template.FuncMap{{
			"add": func(a, b int) int { return a + b },
		}},
	})

	router := httprouter.New()
	router.HandlerFunc(http.MethodGet, "/", func(w http.ResponseWriter, r *http.Request) {
		rend.HTML(w, http.StatusOK, "homepage", nil)
	})
	auth.RegisterRoutes(router)

	config := janus.MustLoadConfig(configFile)
	prh := &provisioner.Handler{
		Config:        config.Provisioner,
		UseSSO:        true,
		EmailFromAddr: emailFromAddr,
		Mattermost:    mmc,
		Gitlab:        glc,
		Mailgun:       mgc,
	}
	router.POST("/user/provision/", prh.Provision)

	usradm := &useradmin.Handler{
		Render:     rend,
		Config:     config.UserAdmin,
		Gitlab:     glc,
		Mattermost: mmc,
	}
	usradm.RegisterRoutes(router, "/user/admin")

	log.Println("Starting server at", listenAddr)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("ListenAndServe(%q) error: %v", listenAddr, err)
	}
}

func env(name, defval string) string {
	if val, found := os.LookupEnv(name); found {
		return val
	}
	return defval
}
