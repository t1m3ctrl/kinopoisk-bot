# Используем официальный golang образ как stage сборки
FROM golang:1.24 as builder

WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd

#RUN go build -o main ./cmd

######## Start a new stage from scratch #######
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/main .
RUN mkdir -p ./configs
COPY ./configs/config.yml ./configs/config.yml
COPY ./static/not-found.png ./static/not-found.png
COPY .env .

# Открываем порт
EXPOSE 8000

# Запускаем приложение
CMD ["./main"]