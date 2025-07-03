package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
)

type Order struct {
	OrderUID          string          `json:"order_uid"`
	TrackNumber       string          `json:"track_number"`
	Entry             string          `json:"entry"`
	Locale            string          `json:"locale"`
	InternalSignature string          `json:"internal_signature"`
	CustomerID        string          `json:"customer_id"`
	DeliveryService   string          `json:"delivery_service"`
	ShardKey          string          `json:"shardkey"`
	SmID              int             `json:"sm_id"`
	DateCreated       time.Time       `json:"date_created"`
	OofShard          string          `json:"oof_shard"`
	Delivery          json.RawMessage `json:"delivery"`
	Payment           Payment         `json:"payment"`
	Items             []Item          `json:"items"`
}

type Payment struct {
	Transaction  string `json:"transaction"`
	RequestID    string `json:"request_id"`
	Currency     string `json:"currency"`
	Provider     string `json:"provider"`
	Amount       int    `json:"amount"`
	PaymentDT    int    `json:"payment_dt"`
	Bank         string `json:"bank"`
	DeliveryCost int    `json:"delivery_cost"`
	GoodsTotal   int    `json:"goods_total"`
	CustomFee    int    `json:"custom_fee"`
}

type Item struct {
	ChrtID      int64  `json:"chrt_id"`
	TrackNumber string `json:"track_number"`
	Price       int    `json:"price"`
	Rid         string `json:"rid"`
	Name        string `json:"name"`
	Sale        int    `json:"sale"`
	Size        string `json:"size"`
	TotalPrice  int    `json:"total_price"`
	NmID        int64  `json:"nm_id"`
	Brand       string `json:"brand"`
	Status      int    `json:"status"`
}

var (
	db          *sql.DB
	kafkaWriter *kafka.Writer
	kafkaReader *kafka.Reader
	cache       = make(map[string]Order)
	cacheMu     sync.RWMutex
)

func main() {
	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	kafkaWriter = &kafka.Writer{
		Addr:  kafka.TCP(os.Getenv("KAFKA_BROKER")),
		Topic: "orders",
	}
	defer kafkaWriter.Close()

	kafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{os.Getenv("KAFKA_BROKER")},
		Topic:     "orders",
		GroupID:   "order-consumer-group",
		Partition: 0,
	})
	defer kafkaReader.Close()

	restoreCacheFromDB()
	go consumeKafka()

	http.HandleFunc("/orders", ordersHandler)
	http.HandleFunc("/order/", orderByIDHandler)
	http.HandleFunc("/", serveIndex)
	log.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func restoreCacheFromDB() {
	rows, err := db.Query(`SELECT order_uid, track_number, entry, locale, internal_signature, customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard, delivery FROM "order"`)
	if err != nil {
		log.Println("restoreCacheFromDB error:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.OrderUID, &o.TrackNumber, &o.Entry, &o.Locale, &o.InternalSignature, &o.CustomerID, &o.DeliveryService, &o.ShardKey, &o.SmID, &o.DateCreated, &o.OofShard, &o.Delivery); err != nil {
			continue
		}
		prow := db.QueryRow(`SELECT transaction, request_id, currency, provider, amount, payment_dt, bank, delivery_cost, goods_total, custom_fee FROM payment WHERE order_uid=$1`, o.OrderUID)
		_ = prow.Scan(&o.Payment.Transaction, &o.Payment.RequestID, &o.Payment.Currency, &o.Payment.Provider, &o.Payment.Amount, &o.Payment.PaymentDT, &o.Payment.Bank, &o.Payment.DeliveryCost, &o.Payment.GoodsTotal, &o.Payment.CustomFee)
		itemRows, _ := db.Query(`SELECT chrt_id, track_number, price, rid, name, sale, size, total_price, nm_id, brand, status FROM item WHERE order_uid=$1`, o.OrderUID)
		for itemRows.Next() {
			var it Item
			_ = itemRows.Scan(&it.ChrtID, &it.TrackNumber, &it.Price, &it.Rid, &it.Name, &it.Sale, &it.Size, &it.TotalPrice, &it.NmID, &it.Brand, &it.Status)
			o.Items = append(o.Items, it)
		}
		itemRows.Close()
		cacheMu.Lock()
		cache[o.OrderUID] = o
		cacheMu.Unlock()
	}
	log.Printf("Cache restored: %d orders", len(cache))
}

func consumeKafka() {
	for {
		m, err := kafkaReader.ReadMessage(context.Background())
		if err != nil {
			log.Println("Kafka read error:", err)
			continue
		}
		var o Order
		if err := json.Unmarshal(m.Value, &o); err != nil {
			log.Println("Invalid order from Kafka:", err)
			continue
		}
		if o.OrderUID == "" {
			log.Println("Order without order_uid, skip")
			continue
		}
		if err := saveOrder(&o); err != nil {
			log.Println("DB error on Kafka order:", err)
			continue
		}
		cacheMu.Lock()
		cache[o.OrderUID] = o
		cacheMu.Unlock()
		log.Printf("Order %s saved from Kafka", o.OrderUID)
	}
}

func orderByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := r.URL.Path[len("/order/"):]
	cacheMu.RLock()
	o, ok := cache[id]
	cacheMu.RUnlock()
	if ok {
		json.NewEncoder(w).Encode(o)
		return
	}
	row := db.QueryRow(`SELECT order_uid, track_number, entry, locale, internal_signature, customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard, delivery FROM "order" WHERE order_uid=$1`, id)
	if err := row.Scan(&o.OrderUID, &o.TrackNumber, &o.Entry, &o.Locale, &o.InternalSignature, &o.CustomerID, &o.DeliveryService, &o.ShardKey, &o.SmID, &o.DateCreated, &o.OofShard, &o.Delivery); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	prow := db.QueryRow(`SELECT transaction, request_id, currency, provider, amount, payment_dt, bank, delivery_cost, goods_total, custom_fee FROM payment WHERE order_uid=$1`, o.OrderUID)
	_ = prow.Scan(&o.Payment.Transaction, &o.Payment.RequestID, &o.Payment.Currency, &o.Payment.Provider, &o.Payment.Amount, &o.Payment.PaymentDT, &o.Payment.Bank, &o.Payment.DeliveryCost, &o.Payment.GoodsTotal, &o.Payment.CustomFee)
	itemRows, _ := db.Query(`SELECT chrt_id, track_number, price, rid, name, sale, size, total_price, nm_id, brand, status FROM item WHERE order_uid=$1`, o.OrderUID)
	for itemRows.Next() {
		var it Item
		_ = itemRows.Scan(&it.ChrtID, &it.TrackNumber, &it.Price, &it.Rid, &it.Name, &it.Sale, &it.Size, &it.TotalPrice, &it.NmID, &it.Brand, &it.Status)
		o.Items = append(o.Items, it)
	}
	itemRows.Close()
	cacheMu.Lock()
	cache[o.OrderUID] = o
	cacheMu.Unlock()
	json.NewEncoder(w).Encode(o)
}

func ordersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var o Order
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		http.Error(w, "invalid input", 400)
		return
	}
	if o.OrderUID == "" {
		http.Error(w, "order_uid required", 400)
		return
	}
	if err := saveOrder(&o); err != nil {
		http.Error(w, "db error", 500)
		return
	}
	cacheMu.Lock()
	cache[o.OrderUID] = o
	cacheMu.Unlock()
	msg, _ := json.Marshal(o)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = kafkaWriter.WriteMessages(ctx, kafka.Message{Value: msg})
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(o)
}

func saveOrder(o *Order) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO "order" (order_uid, track_number, entry, locale, internal_signature, customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard, delivery) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (order_uid) DO NOTHING`,
		o.OrderUID, o.TrackNumber, o.Entry, o.Locale, o.InternalSignature, o.CustomerID, o.DeliveryService, o.ShardKey, o.SmID, o.DateCreated, o.OofShard, o.Delivery)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO payment (transaction, request_id, currency, provider, amount, payment_dt, bank, delivery_cost, goods_total, custom_fee, order_uid) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (transaction) DO NOTHING`,
		o.Payment.Transaction, o.Payment.RequestID, o.Payment.Currency, o.Payment.Provider, o.Payment.Amount, o.Payment.PaymentDT, o.Payment.Bank, o.Payment.DeliveryCost, o.Payment.GoodsTotal, o.Payment.CustomFee, o.OrderUID)
	if err != nil {
		return err
	}
	for _, it := range o.Items {
		_, err = tx.Exec(`INSERT INTO item (track_number, price, rid, name, sale, size, total_price, nm_id, brand, status, order_uid) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			it.TrackNumber, it.Price, it.Rid, it.Name, it.Sale, it.Size, it.TotalPrice, it.NmID, it.Brand, it.Status, o.OrderUID)
		if err != nil {
			return err
		}
	}
	tx.Commit()
	return nil
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	f, err := os.Open(filepath.Join("static", "index.html"))
	if err != nil {
		http.Error(w, "index.html not found", 500)
		return
	}
	defer f.Close()
	io.Copy(w, f)
}
