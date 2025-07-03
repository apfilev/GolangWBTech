package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
	data, err := os.ReadFile("example_order.json")
	if err != nil {
		fmt.Println("Ошибка чтения example_order.json:", err)
		os.Exit(1)
	}

	kafkaURL := os.Getenv("KAFKA_BROKER")
	if kafkaURL == "" || strings.HasPrefix(kafkaURL, "kafka:") {
		kafkaURL = "localhost:9092"
	}
	fmt.Println("Используется Kafka broker:", kafkaURL)

	// Автоматическое создание топика orders
	conn, err := kafka.Dial("tcp", kafkaURL)
	if err != nil {
		fmt.Println("Ошибка подключения к Kafka:", err)
		os.Exit(1)
	}
	defer conn.Close()
	controller, err := conn.Controller()
	if err != nil {
		fmt.Println("Ошибка получения контроллера Kafka:", err)
		os.Exit(1)
	}
	controllerHost := controller.Host
	if controllerHost == "kafka" {
		controllerHost = "localhost"
	}
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controllerHost, controller.Port))
	if err != nil {
		fmt.Println("Ошибка подключения к контроллеру Kafka:", err)
		os.Exit(1)
	}
	defer controllerConn.Close()
	_ = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             "orders",
		NumPartitions:     1,
		ReplicationFactor: 1,
	})

	w := kafka.Writer{
		Addr:  kafka.TCP(kafkaURL),
		Topic: "orders",
	}
	defer w.Close()

	var js json.RawMessage
	if err := json.Unmarshal(data, &js); err != nil {
		fmt.Println("Некорректный JSON:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{Value: data}); err != nil {
		fmt.Println("Ошибка отправки в Kafka:", err)
		os.Exit(1)
	}
	fmt.Println("Сообщение успешно отправлено в Kafka (topic orders)")
}
