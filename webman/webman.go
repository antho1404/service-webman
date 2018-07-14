// Package webman is web application to make and receive POST requests.
package webman

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/tylerb/graceful"
)

// Webman holds information about a webman app.
type Webman struct {
	timeout time.Duration
	client  *http.Client

	webhook *Webhook
	mw      sync.RWMutex

	log *log.Logger
}

// Option is the configuration function for Webman.
type Option func(*Webman)

// New creates a new Webman with given options.
func New(options ...Option) (*Webman, error) {
	w := &Webman{
		timeout: time.Second * 10,
	}
	for _, option := range options {
		option(w)
	}
	if w.log == nil {
		return nil, errors.New("no logger set")
	}
	w.client = &http.Client{
		Timeout: w.timeout,
	}
	return w, nil
}

// TimeoutOption specifies a timeout for unresponsive http calls.
func TimeoutOption(d time.Duration) Option {
	return func(w *Webman) {
		w.timeout = d
	}
}

// LoggerOption used to log webhook logs.
func LoggerOption(l *log.Logger) Option {
	return func(w *Webman) {
		w.log = l
	}
}

// Post performs a http post request to given url with json data.
// out will be filled by response json.
func (w *Webman) Post(url string, data, out interface{}) (statusCode int, err error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return statusCode, err
	}
	resp, err := w.client.Post(url, "application/json", bytes.NewBuffer(dataBytes))
	if err != nil {
		return statusCode, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
}

// Webhook represent a webhook server.
type Webhook struct {
	webman *Webman
	server *graceful.Server
	fn     func(*http.Request) error
}

// StartWebhook starts the webhook server and executes fn for each received call.
func (w *Webman) StartWebhook(endpoint, listenAddr string, fn func(*http.Request) error) error {
	w.webhook = &Webhook{
		webman: w,
		fn:     fn,
	}

	r := mux.NewRouter()
	r.HandleFunc(endpoint, w.webhook.handler).Methods("POST")

	server := &graceful.Server{
		Timeout: w.timeout,
		Server: &http.Server{
			Addr:    listenAddr,
			Handler: r,
		},
	}
	w.mw.Lock()
	w.webhook.server = server
	w.mw.Unlock()

	w.log.Printf("webhook server started at: %s:", listenAddr)
	return w.webhook.server.ListenAndServe()
}

type errorResponse struct {
	Error errorResponseMessage `json:"error"`
}

type errorResponseMessage struct {
	Message string `json:"message"`
}

func (wh *Webhook) handler(w http.ResponseWriter, r *http.Request) {
	if err := wh.fn(r); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		bytes, err := json.Marshal(errorResponse{errorResponseMessage{err.Error()}})
		if err != nil {
			wh.webman.log.Printf("error while encoding error response: %s", err)
			return
		}
		if _, err := w.Write(bytes); err != nil {
			wh.webman.log.Printf("error while sending http response: %s", err)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// WebhookAddr returns server listening address.
func (w *Webman) WebhookAddr() string {
	w.mw.RLock()
	defer w.mw.RUnlock()
	if w.webhook.server != nil {
		return w.webhook.server.Addr
	}
	return ""
}

// ShutdownWebHook closes webhook server.
func (w *Webman) ShutdownWebhook() {
	w.mw.RLock()
	defer w.mw.RUnlock()
	if w.webhook != nil {
		w.webhook.server.Stop(w.timeout)
		<-w.webhook.server.StopChan()
	}
}
