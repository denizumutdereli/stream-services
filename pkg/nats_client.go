package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/nats-io/nats.go"
)

var channel string
var topic string

func main() {
	natsURLs := "nats://127.0.1.1:4222,nats://127.0.1.1:4223,nats://127.0.1.1:4224"
	nc, err := nats.Connect(natsURLs)
	if err != nil {
		log.Fatalf("Error connecting to NATS servers: %v", err)
	}
	defer nc.Close()

	flag.StringVar(&channel, "channel", "markets", "Channel of the service")
	flag.StringVar(&topic, "topic", "ALL", "Topic of the service")
	flag.Parse()
	// // Subscribe to the specific subject
	// _, err = nc.Subscribe("orderbook.*", func(m *nats.Msg) {
	// 	fmt.Printf("Received a message: %s\n", string(m.Data))
	// 	fmt.Println("--------------------------------------------------------------------------------")
	// })
	// if err != nil {
	// 	log.Fatalf("Error subscribing to subject: %v", err)
	// }

	// Define a message handler
	handler := func(m *nats.Msg) {
		fmt.Printf("Received a message: %s\n", string(m.Data))
		fmt.Println("-------------------------------------", m.Subject)
	}

	natsTopic := fmt.Sprintf("%s.%s", strings.ToLower(channel), strings.ToUpper(topic))

	fmt.Println(natsTopic, "--------------------------------------------->>")

	// Subscribe to subject "updates" with queue group "worker-group"
	_, err = nc.QueueSubscribe(natsTopic, "worker-group", handler)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Listening for messages on streaming markets...")
	select {} // Block forever
}
