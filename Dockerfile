# Golang build
# FROM golang:1.23.2-alpine AS go-builder
FROM debian:bullseye AS go-builder
WORKDIR /app

RUN apt-get update -y && apt-get install -y wget gcc libc6-dev sqlite3 libsqlite3-dev git && rm -rf /var/lib/apt/lists/*

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
FROM node:20.12.2 AS react-builder
WORKDIR /app

COPY client/ ./client
COPY .env.client ./client/.env

RUN cd client && npm install && npm run build

# Final image
FROM debian:bullseye-slim
WORKDIR /app

# Install runtime dependencies from project.toml
RUN apt-get update -y && apt-get install -y \
    libatk-bridge2.0-0 \
    curl \
    libatk1.0-0 \
    libdrm2 \
    libgbm1 \
    libasound2 \
    libx11-6 \
    libxcb1 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxkbcommon0 \
    libxrandr2 \
    libatspi2.0-0 \
    sqlite3 \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsS https://dl.brave.com/install.sh | sh
# Copy the Go binary from the first stage
COPY --from=go-builder /app/main /app/main
# Copy the built React app from the second stage
COPY --from=react-builder /app/client/dist /app/client/dist
#COPY ./.env /app/.env

EXPOSE 8000
CMD ["/app/main"]