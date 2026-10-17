//go:build devauth

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// DevAuthEnabled is true only in binaries built with `-tags devauth` (mise run dev, tests).
const DevAuthEnabled = true

func checkDevAuthAllowed(appEnvironment string) error {
	if appEnvironment == "production" {
		return errors.New("auth: this binary was built with the devauth tag and refuses to run with APP_ENV=production")
	}
	return nil
}

// EffectiveSiteOwnerDiscordID lets local work run without a Discord account: with
// SITE_OWNER_DISCORD_ID unset, the seeded Sample DM (seed:dm) is the site owner.
func EffectiveSiteOwnerDiscordID(configured string) string {
	if configured == "" {
		return core.SeedDmDiscordID
	}
	return configured
}

const devNameMaxLength = 60

// sampleCampaignSlug is the campaign `as=dm|player|pending` mean, so the M0 and M1 checklists work.
const sampleCampaignSlug = "sample-colony"

// DevSignIn signs in without Discord (public, devauth builds only):
//
//	/auth/dev?as=owner              the site owner (created on first use)
//	/auth/dev?as=dm                 Sample Colony's DM
//	/auth/dev?as=player             Sample Colony's first active player
//	/auth/dev?as=pending            Sample Colony's first waiting joiner
//	/auth/dev?as=<account id>       any account
//	/auth/dev?as=new&name=<name>    a brand-new person, through the same path as a first Discord sign-in
func (service *Service) DevSignIn() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if fromAnotherSite(request) {
			WriteProblem(writer, http.StatusForbidden, "Dev sign-in works only from this site's own pages or the address bar.")
			return
		}
		requestContext := request.Context()
		query := request.URL.Query()
		accountID, failureStatus, err := service.devSignInAccount(request, query.Get("as"), query.Get("name"))
		switch {
		case failureStatus == http.StatusInternalServerError:
			service.options.Logger.ErrorContext(requestContext, "dev sign-in failed", slog.Any("error", err))
			WriteProblem(writer, failureStatus, "Something went wrong.")
			return
		case err != nil:
			WriteProblem(writer, failureStatus, err.Error())
			return
		}
		if err := service.startSession(writer, request, accountID); err != nil {
			service.options.Logger.ErrorContext(requestContext, "dev sign-in could not start a session", slog.Any("error", err))
			WriteProblem(writer, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		http.Redirect(writer, request, "/", http.StatusSeeOther)
	})
}

// fromAnotherSite spots a sign-in started by another website. Dev sign-in is a GET, which
// CrossOriginProtection doesn't cover, and it changes state: it can add a member, ping Beny Bot and replace
// the browser's session. Browsers label every request with Sec-Fetch-Site ("none" for the address bar);
// curl sends nothing. An Origin naming another host catches older browsers' cross-site fetches.
func fromAnotherSite(request *http.Request) bool {
	switch request.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return true
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsedOrigin, err := url.Parse(origin)
	return err != nil || parsedOrigin.Host != request.Host
}

func (service *Service) devSignInAccount(request *http.Request, as string, name string) (uuid.UUID, int, error) {
	requestContext := request.Context()
	switch as {
	case "new":
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > devNameMaxLength {
			return uuid.Nil, http.StatusBadRequest, errors.New("give the new person a name of 1 to 60 characters: /auth/dev?as=new&name=Brielle")
		}
		suffix := make([]byte, 6)
		_, _ = rand.Read(suffix)
		accountID, err := service.options.Core.SignIn(requestContext, core.DiscordIdentity{
			UserID:      "dev:" + hex.EncodeToString(suffix),
			Username:    strings.ToLower(strings.Join(strings.Fields(name), ".")),
			DisplayName: name,
		})
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, err
		}
		return accountID, 0, nil
	case "owner":
		accountID, found, err := service.options.Core.AccountIDForDiscordUser(requestContext, service.options.SiteOwnerDiscordID)
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, err
		}
		if found {
			return accountID, 0, nil
		}
		// A fresh database has no owner yet; the first dev sign-in as the owner creates them.
		accountID, err = service.options.Core.SignIn(requestContext, core.DiscordIdentity{
			UserID:      service.options.SiteOwnerDiscordID,
			Username:    "owner",
			DisplayName: "The site owner",
		})
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, err
		}
		return accountID, 0, nil
	}

	accounts, err := service.options.Core.DevSignInAccounts(requestContext)
	if err != nil {
		return uuid.Nil, http.StatusInternalServerError, err
	}
	inSample := func(account core.DevSignInAccount, role core.CampaignRole, status core.MemberStatus) bool {
		for _, membership := range account.Memberships {
			if membership.CampaignSlug == sampleCampaignSlug && membership.Role == role && membership.Status == status {
				return true
			}
		}
		return false
	}
	var choose func(core.DevSignInAccount) bool
	switch as {
	case "dm":
		choose = func(account core.DevSignInAccount) bool {
			return inSample(account, core.CampaignRoleDm, core.MemberStatusActive)
		}
	case "player":
		choose = func(account core.DevSignInAccount) bool {
			return inSample(account, core.CampaignRolePlayer, core.MemberStatusActive)
		}
	case "pending":
		choose = func(account core.DevSignInAccount) bool {
			return inSample(account, core.CampaignRolePlayer, core.MemberStatusPending)
		}
	default:
		accountID, parseError := uuid.Parse(as)
		if parseError != nil {
			return uuid.Nil, http.StatusBadRequest, errors.New("use /auth/dev?as=owner|dm|player|pending|<account id>, or ?as=new&name=<name>")
		}
		choose = func(account core.DevSignInAccount) bool { return account.AccountID == accountID }
	}
	for _, account := range accounts {
		if choose(account) {
			return account.AccountID, 0, nil
		}
	}
	return uuid.Nil, http.StatusNotFound, errors.New("no such account here; run `mise run fresh` to load the sample campaigns")
}
