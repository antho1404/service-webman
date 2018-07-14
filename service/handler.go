package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	mesg "github.com/ilgooz/mesg-go"
	uuid "github.com/satori/go.uuid"
)

func (s *Service) webhookHandler(req *http.Request) error {
	var out interface{}
	defer req.Body.Close()
	if err := json.NewDecoder(req.Body).Decode(&out); err != nil {
		return errors.New("json data payload expected")
	}

	err := s.mesgService.EmitEvent("onRequest", webhookResponse{
		Date: time.Now().Unix(),
		ID:   uuid.NewV4().String(),
		Body: out,
	})
	if err != nil {
		log.Printf("error while emitting an event: %s", err)
	}
	return nil
}

type webhookResponse struct {
	Date int64       `json:"date"`
	ID   string      `json:"id"`
	Body interface{} `json:"body"`
}

func (s *Service) executeHandler(req *mesg.Request) {
	var hreq httpRequest

	if err := req.Get(&hreq); err != nil {
		if err := req.Reply("error", httpErrorResponse{
			Message: fmt.Sprintf("err while decoding input data: %s", err),
		}); err != nil {
			log.Printf("error while reply: %s", err)
		}
		return
	}

	responseC := make(chan response, 1)
	s.doPOSTRequest(hreq, responseC)
	resp := <-responseC

	if resp.Error != nil {
		if err := req.Reply("error", httpErrorResponse{
			Message: fmt.Sprintf("err while performing the post request: %s", resp.Error),
		}); err != nil {
			log.Printf("error while reply: %s", err)
		}
		return
	}

	if err := req.Reply("success", httpSuccessResponse{
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}); err != nil {
		log.Printf("error while reply: %s", err)
	}
}

func (s *Service) batchExecuteHandler(req *mesg.Request) {
	var hreq httpBatchRequest

	if err := req.Get(&hreq); err != nil {
		if err := req.Reply("error", httpErrorResponse{
			Message: fmt.Sprintf("err while decoding batch input data: %s", err),
		}); err != nil {
			log.Printf("error while reply: %s", err)
		}
		return
	}

	responseC := make(chan response, 0)

	for _, hreq := range hreq.Batch {
		go s.doPOSTRequest(hreq, responseC)
	}

	hresp := httpBatchResponse{
		Batch: httpBatchResponseBody{
			Successes: map[string]httpSuccessResponse{},
			Errors:    map[string]httpErrorResponse{},
		},
	}

	totalReqs := len(hreq.Batch)
	for i := 0; i < totalReqs; i++ {
		resp := <-responseC

		if resp.Error != nil {
			hresp.Batch.Errors[resp.URL] = httpErrorResponse{
				Message: resp.Error.Error(),
			}
			continue
		}

		hresp.Batch.Successes[resp.URL] = httpSuccessResponse{
			StatusCode: resp.StatusCode,
			Body:       resp.Body,
		}
	}

	if err := req.Reply("batch", hresp); err != nil {
		log.Printf("error while reply: %s", err)
	}
}

func (s *Service) doPOSTRequest(hreq httpRequest, responseC chan response) {
	resp := response{URL: hreq.URL}

	statusCode, err := s.webman.Post(hreq.URL, hreq.Body, &resp.Body)
	if err != nil {
		resp.Error = err
		responseC <- resp
		return
	}

	resp.StatusCode = fmt.Sprintf("%d", statusCode)
	responseC <- resp
}

type httpRequest struct {
	URL  string      `json:"url"`
	Body interface{} `json:"body"`
}

type httpSuccessResponse struct {
	StatusCode string      `json:"statusCode"`
	Body       interface{} `json:"body"`
}

type httpErrorResponse struct {
	Message string `json:"message"`
}

type httpBatchRequest struct {
	Batch []httpRequest `json:"batch"`
}

type httpBatchResponse struct {
	Batch httpBatchResponseBody `json:"batch"`
}

type httpBatchResponseBody struct {
	Successes map[string]httpSuccessResponse `json:"successes"`
	Errors    map[string]httpErrorResponse   `json:"errors"`
}

type response struct {
	URL        string
	StatusCode string
	Body       interface{}
	Error      error
}
