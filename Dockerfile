FROM golang:1.8
WORKDIR /go/src/github.com/ilgooz/service-webman
COPY . .
RUN go install -v ./...
RUN cd /go/bin
CMD ["cmd"]