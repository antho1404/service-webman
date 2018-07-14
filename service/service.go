// Package service is a microservice to handle execute, batchExecute tasks and emit onRequest events.
package service

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	mesg "github.com/ilgooz/mesg-go"
	"github.com/ilgooz/service-webman/webman"
)

type ServiceProvider interface {
	ListenTasks(task mesg.Task, tasks ...mesg.Task) error
	EmitEvent(name string, data interface{}) error
	Close() error
}

type Application interface {
	Post(url string, data, out interface{}) (statusCode int, err error)
	StartWebhook(endpoint, addr string, h func(*http.Request) error) error
	ShutdownWebhook()
}

// Service represents the microservice.
type Service struct {
	mesgService ServiceProvider
	webman      Application

	log       *log.Logger
	logOutput io.Writer

	errC chan error

	webhookEndpoint string
	webhookAddr     string
}

// New creates a Service with given options.
func New(options ...Option) (*Service, error) {
	s := &Service{
		logOutput: os.Stdout,
		errC:      make(chan error, 0),
	}
	for _, option := range options {
		option(s)
	}
	s.log = log.New(s.logOutput, "service-webman: ", log.LstdFlags)

	if s.webhookAddr == "" || s.webhookEndpoint == "" {
		return nil, errors.New("webhook configurations not set")
	}

	var err error

	if s.webman == nil {
		s.webman, err = webman.New(webman.LoggerOption(s.log))
		if err != nil {
			return nil, err
		}
	}

	if s.mesgService == nil {
		s.mesgService, err = mesg.GetService()
	}
	return s, err
}

// Option is the configuration function for Service.
type Option func(*Service)

// WebhookOption provides configurations for webhook server.
func WebhookOption(endpoint, addr string) Option {
	return func(s *Service) {
		s.webhookEndpoint = endpoint
		s.webhookAddr = addr
	}
}

// LogOutputOption uses out as a log destination.
func LogOutputOption(out io.Writer) Option {
	return func(s *Service) {
		s.logOutput = out
	}
}

func serviceProviderOption(provider ServiceProvider) Option {
	return func(s *Service) {
		s.mesgService = provider
	}
}

func applicationServiceOption(app Application) Option {
	return func(s *Service) {
		s.webman = app
	}
}

// Start starts the service and blocks untill there is an error.
func (s *Service) Start() error {
	go s.listenTasks()
	go s.startWebhook()
	err := <-s.errC
	s.Close()
	return err
}

func (s *Service) listenTasks() {
	if err := s.mesgService.ListenTasks(
		mesg.NewTask("execute", s.executeHandler),
		mesg.NewTask("batchExecute", s.batchExecuteHandler),
	); err != nil {
		s.errC <- err
	}
}

func (s *Service) startWebhook() {
	if err := s.webman.StartWebhook(s.webhookEndpoint, s.webhookAddr, s.webhookHandler); err != nil {
		s.errC <- err
	}
}

// Close gracefully closes service.
func (s *Service) Close() error {
	s.webman.ShutdownWebhook()
	s.mesgService.Close()
	return nil
}
