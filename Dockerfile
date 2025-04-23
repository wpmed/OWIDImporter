# Golang build
# FROM golang:1.23.2-alpine as go-builder
FROM debian:bullseye as go-builder
WORKDIR /app

RUN apt-get update && apt-get install -y \
    wget \
    gcc \
    libc6-dev \
    sqlite3 \
    libsqlite3-dev \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install golang
RUN wget https://golang.org/dl/go1.23.2.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.23.2.linux-amd64.tar.gz \
    && rm go1.23.2.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main .

# Use Node for building the React frontend
FROM node:20.12.2 as react-builder
WORKDIR /app

COPY client/ ./client
RUN cd client && npm install && npm run build

# Final image
FROM debian:bullseye-slim
WORKDIR /app

# Copy the Go binary from the first stage
COPY --from=go-builder /app/main /app/main
# Copy the built React app from the second stage
COPY --from=react-builder /app/client/dist /app/client/dist
#COPY ./.env /app/.env

CMD ["/app/main"]
