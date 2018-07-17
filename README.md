# OAuth2 API Proxy for Development

A reverse proxy to be run on development machines that allows transparent access to APIs and other HTTP resources behind an [oauth2_proxy](https://github.com/bitly/oauth2_proxy) authenticating reverse proxy.

After authenticating via an OpenID Connect identity provider in the default browser, the dev proxy transparently adds the authentication cookie used by *oauth2_proxy* to requests directed to the dev proxy.

## Why?

* No need to modify API clients to add credentials
* No need to store/manage credentials on development machines
* Authentication via identity provider
    - No manually managed credentials on proxy (e.g. basic auth htpasswd file)
    - Centralized user managemnent via provider
* Convenience: Authentication in default browser to leverage active identity provider session

## Usage

1. Start the dev proxy: `oauth2-dev-proxy <TARGET BASE URL>`, e.g. `oauth2-dev-proxy https://api-staging.example.com`
2. The dev proxy automatically opens the identity provider's page in your default browser.
3. Log in to your identity provider - if you're already logged in there, authentication may happen without manual interaction.
4. The proxy shows a success browser page including the local HTTP endpoint you can use for accessing the API you provided in step 1.

Example success message in the browser:

```
Authentication complete. You can now use the proxy.

Proxy URL: http://127.0.0.1:8124
Upstream: https://api-staging.example.com

You can close this window.
```

Example output in the shell:

```
2018/07/10 08:29:45 Please authenticate in your browser via our identity provider.
2018/07/10 08:29:46 Authentication complete. You can now use the proxy.

Proxy URL: http://127.0.0.1:8124
Upstream: https://api-staging.example.com

The proxy will automatically terminate in 12 hours (reauthentication required)
```

## Installation

### Binary

Download the binary from the [releases page](https://github.com/FRI-DAY/oauth2-dev-proxy/releases) and make it executable:

    VERSION="0.0.1"; OS="darwin" # or linux or windows
    curl --silent --location https://github.com/FRI-DAY/oauth2-dev-proxy/releases/download/v${VERSION}/oauth2-dev-proxy_${VERSION}_${OS}_amd64 \
      > /usr/local/bin/oauth2-dev-proxy && \
    chmod +x /usr/local/bin/oauth2-dev-proxy

### Source

    go get github.com/FRI-DAY/oauth2-dev-proxy

## oauth2_proxy Setup

You deploy [oauth2_proxy](https://github.com/bitly/oauth2_proxy) as ususal, the only difference is to set a special redirect URL:

    -redirect-url=http://127.0.0.1:8123/oauth2/callback

You also need to allow this redirect URL in your identity provider's configuration.

## Architecture

The following diagram shows HTTP requests made by the different components.

```
  Development machine
+------------------------------------------+     +--------------+     +-----------------+
|                                          |     |              |     |                 |
| API Client  +------->  oauth2-dev-proxy +----> | oauth2-proxy | +-> | Service (API)   |
| (e.g. curl)                ^             |     |              |     |                 |
|                            |             |     +------+-------+     +-----------------+
|                            |             |            |
| Browser  +-----------------+             |     +------v-----------+
|          +-----------------------------------> |                  |
|                                          |     | Identity Provider|
+------------------------------------------+     |                  |
                                                 +------------------+
```

The *dev proxy* uses a trick to allow authentication with the identity provider in the local browser, while making the requests to the *oauth2_proxy* directly, so the *dev proxy* can capture the authentication cookie from *oauth2_proxy*.

The trick requires a separate *oauth2_proxy* instance to be deployed with a redirect URL of `http://127.0.0.1:8123/oauth2/callback`.

From the dev proxy's point of view, the process works like this:

1. Start an HTTP server on port 8123
2. Request `TARGET/oauth2/start`
    - Store the CSRF cookie
    - Note the identity provider URL the proxy redirects to
3. Open the identity provider URL in the local default browser
4. The user authenticates to the identity provider
5. The identity provider redirects to `http://127.0.0.1:8123/oauth2/callback`
6. Forward the `/oauth2/callback` request to the *oauth2_proxy*, including the auth code
    - From the response, note the `_oauth2_proxy` cookie
7. Start a reverse proxy server to the target URL on another port, configured to transparently add the `_oauth2_proxy` cookie to all requests
8. After the cookie expires, shut down the proxy
