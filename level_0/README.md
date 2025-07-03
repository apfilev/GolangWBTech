# Демонстрационный сервис с Kafka, PostgreSQL

## Описание задачи

Сервис реализует:
- Хранение заказов, платежей и товаров в PostgreSQL
- При создании заказа читает его из Kafka
- API для получения заказов


## Эндпоинты

- `GET /order/{order_uid}` — получить заказ по id

## Быстрый старт

1. Перейдите в папку level_0:
   ```sh
   cd level_0
   ```
2. Запустите сервис и инфраструктуру:
   ```sh
   docker compose up --build
   ```
   Это поднимет:
   - PostgreSQL (с миграцией через init.sql)
   - Kafka + Zookeeper
   - Сам сервис (порт 8080)

     ```

## Пример структуры заказа (JSON)

См. файл [example_order.json](./example_order.json) 

**Как использовать:**
1. Перейдите в папку level_0:
   ```sh
   cd level_0
   ```
2. Соберите и запустите скрипт:
   ```sh
   go run send_to_kafka.go
   ```
   или
   ```sh
   go build send_to_kafka.go
   ./send_to_kafka
   ```
3. Скрипт прочитает example_order.json и отправит его в Kafka (topic orders).
