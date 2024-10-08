FROM golang:1.20-alpine

WORKDIR /app

COPY . .

RUN go mod init GoBack2Onedrive
RUN go mod tidy
RUN go clean -cache
RUN go build -o GoBack2Onedrive .

CMD ["/app/GoBack2Onedrive"]
