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