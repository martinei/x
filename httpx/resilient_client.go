package httpx

import (
	"context"
	"github.com/opentracing/opentracing-go"
	"github.com/ory/x/tracing"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/ory/x/logrusx"
)

type resilientOptions struct {
	ctx          context.Context
	c            *http.Client
	l            interface{}
	retryWaitMin time.Duration
	retryWaitMax time.Duration
	retryMax     int
	tracer       opentracing.Tracer
}

func newResilientOptions() *resilientOptions {
	connTimeout := time.Minute
	return &resilientOptions{
		c:            &http.Client{Timeout: connTimeout},
		retryWaitMin: 1 * time.Second,
		retryWaitMax: 30 * time.Second,
		retryMax:     4,
		l:            log.New(io.Discard, "", log.LstdFlags),
	}
}

type ResilientOptions func(o *resilientOptions)

func ResilientClientWithClient(c *http.Client) ResilientOptions {
	return func(o *resilientOptions) {
		o.c = c
	}
}

// ResilientClientWithTracer wraps the http clients transport with a tracing instrumentation
func ResilientClientWithTracer(tracer opentracing.Tracer) ResilientOptions {
	return func(o *resilientOptions) {
		o.tracer = tracer
	}
}

func ResilientClientWithMaxRetry(retryMax int) ResilientOptions {
	return func(o *resilientOptions) {
		o.retryMax = retryMax
	}
}

func ResilientClientWithMinxRetryWait(retryWaitMin time.Duration) ResilientOptions {
	return func(o *resilientOptions) {
		o.retryWaitMin = retryWaitMin
	}
}

func ResilientClientWithMaxRetryWait(retryWaitMax time.Duration) ResilientOptions {
	return func(o *resilientOptions) {
		o.retryWaitMax = retryWaitMax
	}
}

func ResilientClientWithConnectionTimeout(connTimeout time.Duration) ResilientOptions {
	return func(o *resilientOptions) {
		o.c.Timeout = connTimeout
	}
}

func ResilientClientWithLogger(l *logrusx.Logger) ResilientOptions {
	return func(o *resilientOptions) {
		o.l = l
	}
}

func NewResilientClient(opts ...ResilientOptions) *retryablehttp.Client {
	o := newResilientOptions()
	for _, f := range opts {
		f(o)
	}

	if o.tracer != nil {
		o.c.Transport = tracing.RoundTripper(o.tracer, o.c.Transport)
	}

	return &retryablehttp.Client{
		HTTPClient:   o.c,
		Logger:       o.l,
		RetryWaitMin: o.retryWaitMin,
		RetryWaitMax: o.retryWaitMax,
		RetryMax:     o.retryMax,
		CheckRetry:   retryablehttp.DefaultRetryPolicy,
		Backoff:      retryablehttp.DefaultBackoff,
	}
}
