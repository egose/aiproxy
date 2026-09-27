package upstreamhttp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const MaxRedirects = 10

var (
	ErrRedirectOrigin = errors.New("upstream redirect blocked: destination must have the same scheme, hostname, and port as the original request")
	ErrRedirectLimit  = errors.New("upstream redirect blocked: exceeded 10 redirects")
)

func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	guarded := *client
	guarded.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if err := CheckRedirect(next, via); err != nil {
			return err
		}
		if client.CheckRedirect != nil {
			if err := client.CheckRedirect(next, via); err != nil {
				return err
			}
		}
		return CheckRedirect(next, via)
	}
	return guarded.Do(req)
}

func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || !sameOrigin(via[0].URL, req.URL) {
		return ErrRedirectOrigin
	}
	if len(via) > MaxRedirects {
		return ErrRedirectLimit
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return (a.Scheme == "http" || a.Scheme == "https") &&
		strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
