package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/pkg/browser"
)

func main() {
	proxyAddr := "127.0.0.1:8124"

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr,
			"Usage: %s UPSTREAM_URL\n"+
				"Example: %s https://proxy-dev.example.com\n\n"+
				"Note: The oauth2_proxy running at UPSTREAM_URL must be configured with a\n"+
				"      special redirect URL for this development proxy.\n",
			os.Args[0], os.Args[0])
		os.Exit(1)
	}

	upstreamURL, err := url.Parse(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	// We don't need a public suffix list for cookies - we only make requests
	// to a single host, so there's no need to prevent cookies from being
	// sent across domains.
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %v", err)
	}

	client := http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		realCallbackURL, _ := r.URL.Parse(r.URL.String())
		realCallbackURL.Scheme = upstreamURL.Scheme
		realCallbackURL.Host = upstreamURL.Host
		// realCallbackURL.Path = "/oauth2/callback"

		resp, err := client.Get(realCallbackURL.String())
		if err != nil {
			log.Fatalf("Failed to fetch callback URL: %v", err)
			http.Error(w, "Failed to fetch callback URL", http.StatusInternalServerError)
			return
		}

		var authCookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "_oauth2_proxy" {
				authCookie = c
			}
		}

		reverseProxyDirector := func(r *http.Request) {
			r.Host = upstreamURL.Host
			r.URL.Scheme = upstreamURL.Scheme
			r.URL.Host = upstreamURL.Host

			r.AddCookie(authCookie)

			log.Printf("Forwarding request: %v", r.RequestURI)
		}

		proxyHandler := &httputil.ReverseProxy{
			Director: reverseProxyDirector,
		}

		proxyServer := &http.Server{
			Addr:    proxyAddr,
			Handler: proxyHandler,
		}
		go proxyServer.ListenAndServe()

		authCompleteText := fmt.Sprintf("Authentication complete. You can now use the proxy at "+
			"http://%s\n\n"+
			"Upstream: %s\n\n", proxyAddr, upstreamURL)

		w.Write([]byte(authCompleteText + "You can close this window."))

		log.Printf(authCompleteText +
			"The proxy will automatically terminate in 12 hours (reauthentication required)")
	})

	server := &http.Server{
		Addr:    "127.0.0.1:8123",
		Handler: serverMux,
	}
	// TODO check server is actually running to prevent sending auth code to another app
	go server.ListenAndServe()

	resp, err := client.Get(upstreamURL.String() + "/oauth2/start")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	identityProviderURL := resp.Header.Get("Location")
	if identityProviderURL == "" {
		log.Fatalf("Got empty identity provider URL in response: %+v", resp)
	}

	if err := browser.OpenURL(identityProviderURL); err != nil {
		log.Printf("Failed to open browser, please go to %s", identityProviderURL)
	}
	log.Printf("Please authenticate in your browser via our identity provider.")

	// TODO Fetch expiration date from cookie
	time.Sleep(12 * time.Hour)
	log.Printf("12 hours are up, please reauthenticate.")
}
