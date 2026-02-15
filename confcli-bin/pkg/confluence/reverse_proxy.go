package confluence

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"confcli/pkg/logging"
)

// ReverseProxy acts as a man-in-the-middle to capture cookies during authentication
type ReverseProxy struct {
	targetURL  *url.URL
	cookieJar  *cookiejar.Jar
	resultChan chan error
	logger     *logging.Logger
	authDone   chan bool
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a copy of the incoming request to forward to the target
	targetURL := rp.targetURL.ResolveReference(r.URL)
	
	// Create a new request with the target URL
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		rp.logger.Info("Error creating proxy request: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	// Copy headers from the original request
	for header, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}
	
	// Create a custom transport that captures cookies during redirects
	transport := &CookieCaptureTransport{
		Transport: http.DefaultTransport,
		CookieJar: rp.cookieJar,
		Logger:    rp.logger,
		ResultChan: rp.resultChan,
		AuthDone:  rp.authDone,
	}
	
	// Perform the request to the target server with our custom transport
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Capture cookies from redirect responses
			for _, cookie := range req.Cookies() {
				// Store in our enhanced jar
				rp.cookieJar.SetCookies(req.URL, []*http.Cookie{cookie})
				rp.logger.Debug("Captured cookie during redirect: %s=%s", cookie.Name, cookie.Value)
				
				// Check if this is the IDP cookie we're looking for
				if strings.Contains(strings.ToLower(cookie.Name), "idp_last_account") {
					rp.logger.Info("Found idp_last_account cookie: %s", cookie.Name)
					// Signal completion if we found the important cookie
					select {
					case rp.resultChan <- nil:
					default: // Don't block if channel already has a value
					}
					return http.ErrUseLastResponse // Stop following redirects once we have the cookie
				}
			}
			return nil
		},
		Timeout: 30 * time.Second,
	}
	
	resp, err := client.Do(proxyReq)
	if err != nil {
		rp.logger.Info("Error forwarding request: %v", err)
		http.Error(w, "Error contacting target server", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	
	// Capture cookies from the final response
	for _, cookie := range resp.Cookies() {
		// Store in our enhanced jar
		rp.cookieJar.SetCookies(resp.Request.URL, []*http.Cookie{cookie})
		rp.logger.Debug("Captured response cookie: %s=%s", cookie.Name, cookie.Value)
		
		// Check if this is the IDP cookie we're looking for
		if strings.Contains(strings.ToLower(cookie.Name), "idp_last_account") {
			rp.logger.Info("Found idp_last_account cookie: %s", cookie.Name)
			// Signal completion if we found the important cookie
			select {
			case rp.resultChan <- nil:
			default: // Don't block if channel already has a value
			}
		}
	}
	
	// Copy response headers
	for header, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}
	
	// Set status code
	w.WriteHeader(resp.StatusCode)
	
	// Copy response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		rp.logger.Info("Error reading response body: %v", err)
		http.Error(w, "Error reading response", http.StatusInternalServerError)
		return
	}
	
	_, err = w.Write(body)
	if err != nil {
		rp.logger.Info("Error writing response: %v", err)
	}
}

// CookieCaptureTransport wraps an HTTP transport to capture cookies during redirects
type CookieCaptureTransport struct {
	Transport  http.RoundTripper
	CookieJar  *cookiejar.Jar
	Logger     *logging.Logger
	ResultChan chan error
	AuthDone   chan bool
}

func (cct *CookieCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Perform the request
	resp, err := cct.Transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	
	// Capture cookies from the response
	if resp != nil {
		cookies := resp.Cookies()
		cct.Logger.Debug("Captured %d cookies from response to: %s", len(cookies), req.URL.String())
		
		for _, cookie := range cookies {
			cct.Logger.Debug("Response cookie: %s=%s (domain: %s)", cookie.Name, cookie.Value, cookie.Domain)
			
			// Store cookies in our jar
			cct.CookieJar.SetCookies(req.URL, []*http.Cookie{cookie})
			
			// Check if this is the IDP cookie we're looking for
			if strings.Contains(strings.ToLower(cookie.Name), "idp_last_account") {
				cct.Logger.Info("Found idp_last_account cookie in RoundTrip: %s", cookie.Name)
				// Signal completion if we found the important cookie
				select {
				case cct.ResultChan <- nil:
				default: // Don't block if channel already has a value
				}
			}
		}
	}
	
	return resp, err
}