FROM  golang:1.9.2

WORKDIR /go/src/github.com/logzio/logzio-logging-driver-plugin/

COPY . /go/src/github.com/logzio/logzio-logging-driver-plugin/

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

RUN cd /go/src/github.com/logzio/logzio-logging-driver-plugin && go get -v

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/logzio-logging-driver-plugin .

FROM alpine:3.7
RUN apk --no-cache add ca-certificates
COPY --from=0 /bin/logzio-logging-driver-plugin /bin/
WORKDIR /bin/
ENTRYPOINT ["/bin/logzio-logging-driver-plugin"]