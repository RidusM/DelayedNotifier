package queue

import (
	"encoding/json"
	"log"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewRabbitMQ(url string) *RabbitMQ {
	conn, err := amqp.Dial(url)
	if err != nil {
		log.Fatal(err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}

	q, err := ch.QueueDeclare("notifications", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	return &RabbitMQ{conn: conn, channel: ch}
}

func (rmq *RabbitMQ) Publish(n *entity.Notification) error {
	data, _ := json.Marshal(n)
	return rmq.channel.Publish("", "notifications", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        data,
	})
}

func (rmq *RabbitMQ) Consume(handle func(*entity.Notification)) {
	msgs, err := rmq.channel.Consume("notifications", "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		var n Notification
		json.Unmarshal(msg.Body, &n)
		handle(&n)
	}
}
