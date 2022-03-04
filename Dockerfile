# We'll choose the incredibly lightweight
# Go alpine image to work with
FROM docker.io/golang:1.17-alpine3.14 AS builder

# We create an /app directory in which
# we'll put all of our project code
RUN mkdir /app
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

RUN apk update
RUN apk upgrade
RUN apk add make
COPY . /app
# We want to build our application's binary executable
#RUN go get github.com/GeertJohan/go.rice
#RUN go get github.com/GeertJohan/go.rice/rice
#RUN make embed
RUN make build

# the lightweight scratch image we'll
# run our application within
FROM gcr.io/distroless/static AS production
ENV PORT=9091

COPY --from=builder /app/metrics /metrics

USER 998
EXPOSE ${PORT}

ENTRYPOINT [ "/metrics" ]
