# Chatops manager for Operation Uplift

Steps:

1. Enable Bot accounts in Mattermost from
   `System Console > Integrations > Bot Accounts`.

2. Login to Mattermost as system administrator and create a bot account.
   Go to `your team > Integrations > Bot accounts`. Name your new bot account
   and give it `System Admin` role. Copy the auth token, you'll need it in a
   later step.

3. Add the bot to the relevant team.

4. Set relevant environment variables. See src/cmd/chatopssrv/main.go.
   You can put them in a file (data/*.env) and source it.

5. From the repo top-level directory, run:
   `CHATOPS_MATTERMOST_TOKEN=xxx go run src/cmd/chatopssrv/main.go`

6. Now point the NocoDB webhook to the service you just ran, set up the webhook,
   and it should hopefully do something when new records are added. 
   (For now just create users. Rest is WIP.)


## Setting up Mattermost SSO

1. In Mattermost, SiteURL MUST be configured.
2. In Gitlab, go to Menu -> Admin -> Applications -> New.
3. Fill details as described in the Mattermost instructions, EXCEPT: select Trusted (so users don't see prompt to trust the app), for scope select API.
4. Finalize setup in Mattermost.

## Creating Mattermost user with Gitlab SSO

- usernames, emails must match
- authdata is the gitlab user id/number (as a string)
- authtype is 'gitlab'
- emailverified is 't' 

## Setting up Janus SSO

- Gitlab's .well-known/openid-configuration always returns HTTPS URLs, so if TLS is not setup, the sign-in workflow will fail.
- Workaround by serving a copy of the configuration from a static HTTP server (python3 -m http.server) with s/https/http/g and pointing the discovery URL to it.
