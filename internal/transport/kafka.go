package transport

import (
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type KafkaManager struct {
	Brokers       []string
	ConsumerGroup string
	MaxRetry      int
	RetryWait     time.Duration
}

type kafkaManagerInterface interface {
	NewConsumer(topics []string, autoCommit bool) (*kafka.Consumer, error)
	NewProducer() (*kafka.Producer, error)
}

var _ kafkaManagerInterface = (*KafkaManager)(nil)

func NewKafkaManager(brokers []string, consumerGroup string, maxRetry int, retryWait time.Duration) (*KafkaManager, error) {
	return &KafkaManager{
		Brokers:       brokers,
		ConsumerGroup: consumerGroup,
		MaxRetry:      maxRetry,
		RetryWait:     retryWait,
	}, nil
}

func (k *KafkaManager) NewConsumer(topics []string, autoCommit bool) (*kafka.Consumer, error) {
	var consumer *kafka.Consumer
	var err error

	for i := 0; i < k.MaxRetry; i++ {
		consumer, err = kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers":  k.Brokers,
			"group.id":           k.ConsumerGroup,
			"auto.offset.reset":  "earliest",
			"enable.auto.commit": autoCommit,
		})

		if err != nil {
			fmt.Printf("Failed to create consumer, retry %d/%d\n", i+1, k.MaxRetry)
			time.Sleep(k.RetryWait)
		} else {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create consumer after %d retries: %v", k.MaxRetry, err)
	}

	for i := 0; i < k.MaxRetry; i++ {
		err = consumer.SubscribeTopics(topics, nil)
		if err != nil {
			fmt.Printf("Failed to subscribe to topics, retry %d/%d\n", i+1, k.MaxRetry)
			time.Sleep(k.RetryWait)
		} else {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to topics after %d retries: %v", k.MaxRetry, err)
	}

	fmt.Println("Consumer created and subscribed to topics:", topics)
	return consumer, nil
}

func (k *KafkaManager) NewProducer() (*kafka.Producer, error) {
	var producer *kafka.Producer
	var err error

	for i := 0; i < k.MaxRetry; i++ {
		producer, err = kafka.NewProducer(&kafka.ConfigMap{
			"bootstrap.servers": k.Brokers,
		})

		if err != nil {
			fmt.Printf("Failed to create producer, retry %d/%d\n", i+1, k.MaxRetry)
			time.Sleep(k.RetryWait)
		} else {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create producer after %d retries: %v", k.MaxRetry, err)
	}

	fmt.Println("Producer created")
	return producer, nil
}
