package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"time"

	"golang.org/x/net/publicsuffix"
)

//https://github.com/pkg/browser

func main() {
	proxyAddr := ":8124"

	baseURL, err := url.Parse("https://SETME")
	if err != nil {
		log.Fatalf("Failed to parse baseURL: %v", err)
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
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
		realCallbackURL.Scheme = baseURL.Scheme
		realCallbackURL.Host = baseURL.Host
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

		w.Write([]byte(fmt.Sprintf(
			"Authentication complete. You can now use the proxy at %s\n\n"+
				"Upstream: %s\n\n"+
				"You can close this window.",
			proxyAddr, baseURL)))
		log.Printf("Cookie: %+v", authCookie)
		log.Printf("Response: %+v", resp)

		reverseProxyDirector := func(r *http.Request) {
			log.Printf("Original request: %+v", r)
			r.Host = baseURL.Host
			r.URL.Scheme = baseURL.Scheme
			r.URL.Host = baseURL.Host

			r.AddCookie(authCookie)

			log.Printf("Forwarding request: %+v", r)
			log.Printf("Request URI: %v", r.RequestURI)
		}

		proxyHandler := &httputil.ReverseProxy{
			Director: reverseProxyDirector,
		}

		proxyServer := &http.Server{
			Addr:    proxyAddr,
			Handler: proxyHandler,
		}
		go proxyServer.ListenAndServe()

	})

	server := &http.Server{
		Addr:    ":8123",
		Handler: serverMux,
	}
	go server.ListenAndServe()

	resp, err := client.Get(baseURL.String() + "/oauth2/start")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	identityProviderURL := resp.Header.Get("Location")
	if identityProviderURL == "" {
		log.Fatalf("Got empty identity provider URL in response: %+v", resp)
	}

	log.Printf("Go to %s", identityProviderURL)
	time.Sleep(1 * time.Hour)
}
