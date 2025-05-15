FROM golang:1.24-alpine
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -mod=mod -o hll-geofences ./cmd/cmd.go
CMD ["./hll-geofences"]
