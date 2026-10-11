package auth

import (
	"net"
	"net/http"
	"strings"
)

// RequireKnownHost answers 421 to a request whose Host this server doesn't serve. It applies outside
// production only. A local server is reachable from this machine alone, and a devauth build lets anyone
// sign in as anyone, so a site that re-points its own domain at 127.0.0.1 (DNS rebinding) would pass
// as same-origin. Production serves the public internet, where that trick gains nothing.
//
// Allowed: localhost, IP literals (dev:phone reaches Go through Vite with the Wi-Fi address as Host;
// an attacker can't rebind a name onto an address that is already a number), and PUBLIC_URL's host.
func (service *Service) RequireKnownHost(next http.Handler) http.Handler {
	if service.options.AppEnvironment == "production" {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !service.knownHost(request.Host) {
			WriteProblem(writer, http.StatusMisdirectedRequest, "This server answers only on localhost, an IP address or PUBLIC_URL.")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (service *Service) knownHost(hostAndPort string) bool {
	host := hostAndPort
	if splitHost, _, err := net.SplitHostPort(hostAndPort); err == nil {
		host = splitHost
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	switch {
	case host == "":
		return false
	case host == "localhost":
		return true
	case net.ParseIP(host) != nil:
		return true
	default:
		return host == service.publicHost
	}
}
