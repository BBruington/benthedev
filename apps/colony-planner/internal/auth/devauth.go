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

// EffectiveDmDiscordID lets local work run without a Discord account: with DM_DISCORD_ID unset, the
// seeded sample DM is the DM.
func EffectiveDmDiscordID(configured string) string {
	if configured == "" {
		return core.SeedDmDiscordID
	}
	return configured
}

const devNameMaxLength = 60

// DevSignIn signs in without Discord (public, devauth builds only):
//
//	/auth/dev?as=dm                 the DM (created on first use)
//	/auth/dev?as=player             the first active player
//	/auth/dev?as=pending            the first member waiting for the DM
//	/auth/dev?as=<member id>        any member, whatever their status
//	/auth/dev?as=new&name=<name>    a brand-new person, through the same path as a first Discord sign-in
func (service *Service) DevSignIn() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if fromAnotherSite(request) {
			WriteProblem(writer, http.StatusForbidden, "Dev sign-in works only from this site's own pages or the address bar.")
			return
		}
		requestContext := request.Context()
		query := request.URL.Query()
		memberID, failureStatus, err := service.devSignInMember(request, query.Get("as"), query.Get("name"))
		switch {
		case errors.Is(err, core.ErrSignUpsFull):
			http.Redirect(writer, request, loginPageSignUpsFull, http.StatusSeeOther)
			return
		case failureStatus == http.StatusInternalServerError:
			service.options.Logger.ErrorContext(requestContext, "dev sign-in failed", slog.Any("error", err))
			WriteProblem(writer, failureStatus, "Something went wrong.")
			return
		case err != nil:
			WriteProblem(writer, failureStatus, err.Error())
			return
		}
		if err := service.startSession(writer, request, memberID); err != nil {
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

func (service *Service) devSignInMember(request *http.Request, as string, name string) (uuid.UUID, int, error) {
	requestContext := request.Context()
	if as == "new" {
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > devNameMaxLength {
			return uuid.Nil, http.StatusBadRequest, errors.New("give the new player a name of 1 to 60 characters: /auth/dev?as=new&name=Brielle")
		}
		suffix := make([]byte, 6)
		_, _ = rand.Read(suffix)
		identity := core.DiscordIdentity{
			UserID:      "dev:" + hex.EncodeToString(suffix),
			Username:    strings.ToLower(strings.Join(strings.Fields(name), ".")),
			DisplayName: name,
		}
		memberID, err := service.options.Core.SignIn(requestContext, identity)
		if errors.Is(err, core.ErrSignUpsFull) {
			return uuid.Nil, http.StatusConflict, err
		}
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, err
		}
		return memberID, 0, nil
	}

	members, err := service.options.Core.DevSignInMembers(requestContext)
	if err != nil {
		return uuid.Nil, http.StatusInternalServerError, err
	}
	var choose func(core.DevSignInMember) bool
	switch as {
	case "dm":
		choose = func(member core.DevSignInMember) bool { return member.IsDM }
	case "player":
		choose = func(member core.DevSignInMember) bool {
			return !member.IsDM && member.Status == core.MemberStatusActive
		}
	case "pending":
		choose = func(member core.DevSignInMember) bool { return member.Status == core.MemberStatusPending }
	default:
		memberID, parseError := uuid.Parse(as)
		if parseError != nil {
			return uuid.Nil, http.StatusBadRequest, errors.New("use /auth/dev?as=dm|player|pending|<member id>, or ?as=new&name=<name>")
		}
		choose = func(member core.DevSignInMember) bool { return member.MemberID == memberID }
	}
	for _, member := range members {
		if choose(member) {
			return member.MemberID, 0, nil
		}
	}
	if as == "dm" {
		// A fresh database has no DM yet; the first dev sign-in as the DM creates them.
		memberID, err := service.options.Core.SignIn(requestContext, core.DiscordIdentity{
			UserID:      service.options.DmDiscordID,
			Username:    "dm",
			DisplayName: "The DM",
		})
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, err
		}
		return memberID, 0, nil
	}
	return uuid.Nil, http.StatusNotFound, errors.New("no such member here; run `mise run fresh` to load the sample members")
}
