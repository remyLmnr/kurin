package http

import (
	"fmt"
	"net/http"
	"time"

	"context"

	"os"

	"github.com/maxperrimond/kurin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type (
	Adapter struct {
		srv       *http.Server
		port      int
		version   string
		healthy   bool
		logger    kurin.Logger
		lastError error
		onStop    chan os.Signal
	}
)

func NewHTTPAdapter(handler http.Handler, port int, version string, logger kurin.Logger) kurin.Adapter {
	adapter := &Adapter{
		port:    port,
		version: version,
		healthy: true,
		logger:  logger,
	}

	totalCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_requests_total",
			Help: "A counter for requests to the wrapped handler.",
		},
		[]string{"code", "method"},
	)
	durationHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "app_response_duration_seconds",
			Help:    "A histogram of request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code", "method"},
	)
	prometheus.MustRegister(totalCount, durationHist)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if adapter.healthy {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(adapter.lastError.Error()))
		}
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, version)
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/",
		promhttp.InstrumentHandlerCounter(totalCount,
			promhttp.InstrumentHandlerDuration(durationHist, handler)))

	adapter.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return adapter
}

func (adapter *Adapter) Open() {
	adapter.logger.Info(fmt.Sprintf("Listening on http://0.0.0.0:%d", adapter.port))
	if err := adapter.srv.ListenAndServe(); err != nil {
		adapter.logger.Fatal(err)
	}
}

func (adapter *Adapter) Close() {
	if err := adapter.srv.Shutdown(context.Background()); err != nil {
		adapter.logger.Error(err)
	}
}

func (adapter *Adapter) NotifyStop(c chan os.Signal) {
	adapter.onStop = c
}

func (adapter *Adapter) OnFailure(err error) {
	if err != nil {
		adapter.lastError = err
		adapter.healthy = false
	}
}
