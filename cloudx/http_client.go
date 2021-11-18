package cloudx

import (
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

type tokenTransporter struct {
	http.RoundTripper
	token string
}

func (t *tokenTransporter) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.RoundTripper.RoundTrip(req)
}

func NewHTTPClient() *http.Client {
	c := retryablehttp.NewClient()
	c.Logger = nil
	return c.StandardClient()
}

func NewCloudHTTPClient(token string) *http.Client {
	c := retryablehttp.NewClient()
	c.Logger = nil
	return &http.Client{
		Transport: &tokenTransporter{
			RoundTripper: c.StandardClient().Transport,
			token:        token,
		},
		Timeout: time.Second * 15,
	}
}
