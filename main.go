package main

import (
	"context"
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

	authCompleteNotice := fmt.Sprintf("Authentication complete. You can now use the proxy.\n\n"+
		"Proxy URL: http://%s\n"+
		"Upstream: %s\n\n", proxyAddr, upstreamURL)

	authCookie := authenticate(upstreamURL, authCompleteNotice)

	go runProxy(proxyAddr, upstreamURL, authCookie)

	log.Printf("The proxy will automatically terminate in 12 hours (reauthentication required)")

	// TODO Fetch expiration date from cookie
	time.Sleep(12 * time.Hour)
	log.Printf("12 hours are up, please reauthenticate.")
}

func authenticate(upstreamURL *url.URL, authCompleteNotice string) *http.Cookie {
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

	authCookieCh := make(chan *http.Cookie)

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		realCallbackURL, _ := r.URL.Parse(r.URL.String())
		realCallbackURL.Scheme = upstreamURL.Scheme
		realCallbackURL.Host = upstreamURL.Host

		resp, err := client.Get(realCallbackURL.String())
		if err != nil {
			log.Fatalf("Failed to fetch callback URL: %v", err)
			http.Error(w, "Failed to fetch callback URL", http.StatusInternalServerError)
			return
		}

		for _, c := range resp.Cookies() {
			if c.Name == "_oauth2_proxy" {
				w.Write([]byte(authCompleteNotice + "You can close this window."))
				authCookieCh <- c
				break
			}
		}
	})

	server := &http.Server{
		Addr:    "127.0.0.1:8123",
		Handler: serverMux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start auth callback server: %v", err)
		}
	}()

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

	// Wait for authentication callback, see handler above
	authCookie := <-authCookieCh

	ctx, ctxCancelFn := context.WithTimeout(context.Background(), 1*time.Second)
	_ = server.Shutdown(ctx)
	ctxCancelFn()

	log.Printf(authCompleteNotice)

	return authCookie
}

func runProxy(addr string, upstreamURL *url.URL, authCookie *http.Cookie) {
	director := func(r *http.Request) {
		r.Host = upstreamURL.Host
		r.URL.Scheme = upstreamURL.Scheme
		r.URL.Host = upstreamURL.Host

		r.AddCookie(authCookie)

		log.Printf("Forwarding request: %v", r.RequestURI)
	}

	handler := &httputil.ReverseProxy{
		Director: director,
	}

	proxyServer := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	if err := proxyServer.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start reverse proxy: %v", err)
	}
}
