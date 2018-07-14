package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	mesg "github.com/ilgooz/mesg-go"
	"github.com/mesg-foundation/core/api/service"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var errClosedConn = errors.New("closed connection")

const token = "token"
const endpoint = "endpoint"

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

	srv, err := mesg.NewService(
		mesg.ServiceTokenOption(token),
		mesg.ServiceEndpointOption(endpoint),
	)
	assert.Nil(t, err)
	assert.NotNil(t, srv)

	taskC := make(chan *service.TaskData, 0)
	emitC := make(chan *service.EmitEventRequest, 0)
	srv.Client = &testClient{
		emitC:  emitC,
		stream: &taskDataStream{taskC: taskC},
	}

	tw := &testWebman{startC: make(chan struct{}, 0)}

	s, err := New(
		LogOutputOption(ioutil.Discard),
		WebhookOption("test", "test"),
		mesgServiceOption(srv),
		applicationServiceOption(tw),
	)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Start()
	<-tw.startC

	req, err := http.NewRequest("", "", bytes.NewBuffer(dataBytes))
	assert.Nil(t, err)
	go tw.webhookHandler(req)
	ed := <-emitC
	assert.Equal(t, "onRequest", ed.EventKey)
	var out webhookResponse
	assert.Nil(t, json.Unmarshal([]byte(ed.EventData), &out))
	assert.Equal(t, data, out.Body)
	_, err = uuid.FromString(out.ID)
	assert.NotEmpty(t, out.Date)
}

func TestTasks(t *testing.T) {
	taskKey := "execute"
	taskBatchKey := "batchExecute"
	executionID := "executionID"
	postPayload := map[string]interface{}{"test": "test"}
	statusCode := http.StatusOK
	inputData := httpRequest{
		URL:  "http://mesg.com",
		Body: postPayload,
	}
	inputDataBatch := httpBatchRequest{
		Batch: []httpRequest{
			httpRequest{
				URL:  "http://mesg.com",
				Body: postPayload,
			}, httpRequest{
				URL:  "http://mesg.tech",
				Body: postPayload,
			}},
	}
	inputDataBytes, err := json.Marshal(inputData)
	assert.Nil(t, err)
	inputDataBatchBytes, err := json.Marshal(inputDataBatch)
	assert.Nil(t, err)

	srv, err := mesg.NewService(
		mesg.ServiceTokenOption(token),
		mesg.ServiceEndpointOption(endpoint),
	)
	assert.Nil(t, err)
	assert.NotNil(t, srv)

	taskC := make(chan *service.TaskData, 0)
	submitC := make(chan *service.SubmitResultRequest, 0)
	srv.Client = &testClient{
		stream:  &taskDataStream{taskC: taskC},
		submitC: submitC,
	}
	tw := &testWebman{
		payload:    postPayload,
		statusCode: statusCode,
		startC:     make(chan struct{}, 0),
	}

	s, err := New(
		LogOutputOption(ioutil.Discard),
		WebhookOption("test", "test"),
		mesgServiceOption(srv),
		applicationServiceOption(tw),
	)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Start()

	taskC <- &service.TaskData{
		ExecutionID: executionID,
		TaskKey:     taskKey,
		InputData:   string(inputDataBytes),
	}

	reply := <-submitC
	assert.Equal(t, "success", reply.OutputKey)
	var out httpSuccessResponse
	assert.Nil(t, json.Unmarshal([]byte(reply.OutputData), &out))
	assert.Equal(t, statusCode, out.StatusCode)
	assert.Equal(t, postPayload, out.Body)

	taskC <- &service.TaskData{
		ExecutionID: executionID,
		TaskKey:     taskBatchKey,
		InputData:   string(inputDataBatchBytes),
	}

	reply = <-submitC
	assert.Equal(t, "batch", reply.OutputKey)
	var outBatch httpBatchResponse
	assert.Nil(t, json.Unmarshal([]byte(reply.OutputData), &outBatch))
	assert.Equal(t, len(inputDataBatch.Batch), len(outBatch.Batch.Successes))
	for _, data := range inputDataBatch.Batch {
		assert.Equal(t, statusCode, outBatch.Batch.Successes[data.URL].StatusCode)
		assert.Equal(t, postPayload, outBatch.Batch.Successes[data.URL].Body)
	}
}

type testServiceProvider struct {
	service *mesg.Service
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
	payload         interface{}
	statusCode      int
	webhookEndpoint string
	webhookAddr     string
	webhookHandler  func(*http.Request) error
}

func (tw *testWebman) Post(url string, data, out interface{}) (statusCode int, err error) {
	bytes, err := json.Marshal(tw.payload)
	if err != nil {
		return statusCode, err
	}
	return tw.statusCode, json.Unmarshal(bytes, out)
}

func (tw *testWebman) StartWebhook(endpoint, addr string, h func(*http.Request) error) error {
	tw.webhookEndpoint = endpoint
	tw.webhookAddr = addr
	tw.webhookHandler = h
	tw.startC <- struct{}{}
	return nil
}

func (tw *testWebman) ShutdownWebhook() {}

type testClient struct {
	stream  service.Service_ListenTaskClient
	emitC   chan *service.EmitEventRequest
	submitC chan *service.SubmitResultRequest
}

func (t *testClient) EmitEvent(ctx context.Context, in *service.EmitEventRequest,
	opts ...grpc.CallOption) (*service.EmitEventReply, error) {
	t.emitC <- in
	return nil, nil
}

func (t *testClient) ListenTask(ctx context.Context,
	in *service.ListenTaskRequest,
	opts ...grpc.CallOption) (service.Service_ListenTaskClient, error) {
	return t.stream, nil
}

func (t *testClient) SubmitResult(ctx context.Context,
	in *service.SubmitResultRequest,
	opts ...grpc.CallOption) (*service.SubmitResultReply, error) {
	t.submitC <- in
	return nil, nil
}

type taskDataStream struct {
	taskC chan *service.TaskData
	grpc.ClientStream
}

func (s taskDataStream) Recv() (*service.TaskData, error) {
	data, ok := <-s.taskC
	if !ok {
		return nil, errClosedConn
	}
	return data, nil
}
