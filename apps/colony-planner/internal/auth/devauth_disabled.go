//go:build !devauth

package auth

import (
	"net/http"
)

const DevAuthEnabled = false

func checkDevAuthAllowed(string) error {
	return nil
}

func EffectiveDmDiscordID(configured string) string {
	return configured
}

// DevSignIn is nil without the devauth tag, so /auth/dev is never registered in production builds.
func (service *Service) DevSignIn() http.Handler {
	return nil
}
