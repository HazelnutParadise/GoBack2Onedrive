FROM golang:1.20-alpine

WORKDIR /app

COPY . .

RUN go mod init GoBack2Onedrive
RUN go build -o GoBack2Onedrive .

CMD ["GoBack2Onedrive"]
