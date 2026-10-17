package auth

import (
	"net"
	"net/http"
	"strings"
)

// railwayHealthcheckHost is the Host Railway's deploy health check sends (docs.railway.com/reference/healthchecks).
const railwayHealthcheckHost = "healthcheck.railway.app"

// RequireKnownHost answers 421 to a request whose Host this server doesn't serve.
//
// Production answers only PUBLIC_URL's host: sign-in, its cookie and CrossOriginProtection belong to
// that host, so any other name routed here (the *.up.railway.app address once the domain moves, or a
// stranger's domain pointed at the service) gets nothing. Railway's deploy health check may read
// /healthz, which returns no data.
//
// Elsewhere a local server is reachable from this machine alone, and a devauth build lets anyone sign
// in as anyone, so a site that re-points its own domain at 127.0.0.1 (DNS rebinding) would pass as
// same-origin. Allowed there: localhost, IP literals (dev:phone reaches Go through Vite with the Wi-Fi
// address as Host; an attacker can't rebind a name onto an address that is already a number), and
// PUBLIC_URL's host.
func (service *Service) RequireKnownHost(next http.Handler) http.Handler {
	message := "This server answers only on localhost, an IP address or PUBLIC_URL."
	if service.options.AppEnvironment == "production" {
		message = "This site answers only at its own address."
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !service.knownHost(request) {
			WriteProblem(writer, http.StatusMisdirectedRequest, message)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (service *Service) knownHost(request *http.Request) bool {
	host := request.Host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	if service.options.AppEnvironment == "production" {
		healthCheck := host == railwayHealthcheckHost && request.Method == http.MethodGet && request.URL.Path == "/healthz"
		return host == service.publicHost || healthCheck
	}
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
