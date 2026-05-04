FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN go mod init devbot

RUN go get gopkg.in/telebot.v3
RUN go get github.com/biter777/countries

RUN go mod tidy

RUN go build -o bot main.go

CMD ["./bot"]
