package webman

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/phayes/freeport"
	"github.com/stretchr/testify/assert"
)

var logger = log.New(ioutil.Discard, "webman: ", log.LstdFlags)

type postRequest struct {
	Message string
}

func TestPost(t *testing.T) {
	statusCode := 200
	data := postRequest{"data"}
	dataBytes, err := json.Marshal(data)
	assert.Nil(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write(dataBytes)
	}))
	defer ts.Close()

	w, err := New(LoggerOption(logger))
	assert.Nil(t, err)
	assert.NotNil(t, w)

	var out postRequest
	statusCode1, err := w.Post(ts.URL, data, &out)
	assert.Nil(t, err)
	assert.Equal(t, statusCode, statusCode1)
	assert.Equal(t, data.Message, out.Message)
}

func TestWebhook(t *testing.T) {
	endpoint := "/endpoint"
	statusCode := http.StatusAccepted
	data := postRequest{"data"}
	dataBytes, err := json.Marshal(data)
	port, err := freeport.GetFreePort()
	assert.Nil(t, err)
	listenAddr := fmt.Sprintf(":%d", port)

	w, err := New(LoggerOption(logger))
	assert.Nil(t, err)
	assert.NotNil(t, w)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		assert.Nil(t, w.StartWebhook(endpoint, listenAddr, func(req *http.Request) error {
			assert.NotNil(t, req)
			defer req.Body.Close()
			var out postRequest
			assert.Nil(t, json.NewDecoder(req.Body).Decode(&out))
			assert.Equal(t, data.Message, out.Message)
			return nil
		}))
		wg.Done()
	}()
	time.Sleep(time.Millisecond * 100)

	url := fmt.Sprintf("http://127.0.0.1%s%s", w.WebhookAddr(), endpoint)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(dataBytes))
	assert.Nil(t, err)
	assert.Equal(t, statusCode, resp.StatusCode)
	w.ShutdownWebhook()

	wg.Wait()
}

func TestWebhookErrorCode(t *testing.T) {
	endpoint := "/endpoint"
	statusCode := http.StatusBadRequest
	port, err := freeport.GetFreePort()
	assert.Nil(t, err)
	listenAddr := fmt.Sprintf(":%d", port)

	w, err := New(LoggerOption(logger))
	assert.Nil(t, err)
	assert.NotNil(t, w)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		assert.Nil(t, w.StartWebhook(endpoint, listenAddr, func(req *http.Request) error {
			return errors.New("")
		}))
		wg.Done()
	}()
	time.Sleep(time.Millisecond * 100)

	url := fmt.Sprintf("http://127.0.0.1%s%s", w.WebhookAddr(), endpoint)
	resp, err := http.Post(url, "application/json", nil)
	assert.Nil(t, err)
	assert.Equal(t, statusCode, resp.StatusCode)
	w.ShutdownWebhook()

	wg.Wait()
}
