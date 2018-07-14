package service

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	mesg "github.com/ilgooz/mesg-go"
	"github.com/stretchr/testify/assert"
)

type emitData struct {
	name string
	data interface{}
}

type listenData struct {
	task  mesg.Task
	tasks []mesg.Task
}

type testBody struct {
	io.Reader
}

func (tb *testBody) Close() error {
	return nil
}

func TestOnRequestEvent(t *testing.T) {
	data := map[string]interface{}{"body": "test"}
	dataBytes, err := json.Marshal(data)
	assert.Nil(t, err)

	tp := &testServiceProvider{emitC: make(chan emitData, 0)}
	tw := &testWebman{startC: make(chan struct{}, 0)}

	s, err := New(
		LogOutputOption(ioutil.Discard),
		WebhookOption("test", "test"),
		serviceProviderOption(tp),
		applicationServiceOption(tw),
	)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Start()
	<-tw.startC

	req, err := http.NewRequest("", "", bytes.NewBuffer(dataBytes))
	assert.Nil(t, err)
	go tw.webhookHandler(req)
	ed := <-tp.emitC
	assert.Equal(t, "onRequest", ed.name)
	resp := ed.data.(webhookResponse)
	assert.Equal(t, data, resp.Body)
}

type testServiceProvider struct {
	emitC   chan emitData
	listenC chan listenData
}

func (tp *testServiceProvider) ListenTasks(task mesg.Task, tasks ...mesg.Task) error {
	tp.listenC <- listenData{task, tasks}
	return nil
}

func (tp *testServiceProvider) EmitEvent(name string, data interface{}) error {
	tp.emitC <- emitData{name, data}
	return nil

}
func (tp *testServiceProvider) Close() error {
	return nil
}

type testWebman struct {
	startC          chan struct{}
	webhookEndpoint string
	webhookAddr     string
	webhookHandler  func(*http.Request) error
}

func (tw *testWebman) Post(url string, data, out interface{}) (statusCode int, err error) {
	return 0, nil
}

func (tw *testWebman) StartWebhook(endpoint, addr string, h func(*http.Request) error) error {
	tw.webhookEndpoint = endpoint
	tw.webhookAddr = addr
	tw.webhookHandler = h
	tw.startC <- struct{}{}
	return nil
}

func (tw *testWebman) ShutdownWebhook() {}
