package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/joho/godotenv"
	"github.com/streadway/amqp"
)

type AMQPMessage struct {
	Action    string    `json:"action"`
	Type      string    `json:"type"`
	Receiver  string    `json:"receiver"`
	Message   string    `json:"message"`
	Code      string    `json:"code"`
	TTL       int       `json:"ttl"`
	SendAt    time.Time `json:"sendAt"`
	CreatedAt int       `json:"createdAt"`
	UUID      string    `json:"uuid"`
	Realm     string    `json:"realm"`
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
		os.Exit(0)
	}
}

var re = regexp.MustCompile(`^(?:(?:\(?(?:00|\+)([1-4]\d\d|[1-9]\d?)\)?)?[\-\.\ \\\/]?)?((?:\(?\d{1,}\)?[\-\.\ \\\/]?){0,})(?:[\-\.\ \\\/]?(?:#|ext\.?|extension|x)[\-\.\ \\\/]?(\d+))?$`)

func validateMobileNumber(number string) bool {
	log.Printf("Validating mobile number: %s\n", number)
	return re.MatchString(number)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file loaded")
	}

	amqpConnectionString := os.Getenv("AMQP_CONNECTION")
	amqpQueue := os.Getenv("AMQP_QUEUE")
	amqpQueueEvents := os.Getenv("AMQP_QUEUE_EVENTS")
	amqpQueueEventsAdmin := os.Getenv("AMQP_QUEUE_EVENTS_ADMIN")
	log.Printf("ENV AMQP CONNECTION: %s\n", amqpConnectionString)
	log.Printf("ENV AMQP QUEUE: %s\n", amqpQueue)

	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("eu-west-1"))
	if err != nil {
		failOnError(err, "unable to load SDK config")

	}

	client := sns.NewFromConfig(cfg)

	conn, err := amqp.Dial(amqpConnectionString)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		amqpQueue, // name
		false,     // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	failOnError(err, "Failed to declare a queue")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			log.Printf("Received encoded AMQP message: %s\n", d.Body)
			var amqpMessage AMQPMessage
			if err := json.Unmarshal(d.Body, &amqpMessage); err != nil {
				log.Println(err)
				continue
			}
			log.Printf("Decoded AMQP Message: %+v\n----\n", amqpMessage)
			if validateMobileNumber(amqpMessage.Receiver) == false {
				log.Println("--> Validating mobile did not suceed")
				continue
			}

			params := &sns.PublishInput{
				Message:     aws.String(amqpMessage.Message),
				PhoneNumber: aws.String(amqpMessage.Receiver),
				MessageAttributes: map[string]types.MessageAttributeValue{
					"AWS.SNS.SMS.SenderID": {
						DataType:    aws.String("String"),
						StringValue: aws.String("MATSIAM"),
					},
					"AWS.SNS.SMS.SMSType": {
						DataType:    aws.String("String"),
						StringValue: aws.String("Transactional"),
					},
				},
			}
			resp, err := client.Publish(context.TODO(), params)

			if err != nil {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				log.Printf("-->Error sending SMS: %s\n", err.Error())
				continue
			}

			// Pretty-print the response data.
			log.Printf("--> Success sending SMS: %s\n", *resp.MessageId)
		}
	}()

	log.Printf(" [*] Waiting for notification messages")

	if len(amqpQueueEvents) > 0 {
		qEvents, err := ch.QueueDeclare(
			amqpQueueEvents, // name
			false,           // durable
			false,           // delete when unused
			false,           // exclusive
			false,           // no-wait
			nil,             // arguments
		)
		if err == nil {
			msgsEvents, err := ch.Consume(
				qEvents.Name, // queue
				"",           // consumer
				true,         // auto-ack
				false,        // exclusive
				false,        // no-local
				false,        // no-wait
				nil,          // args
			)
			if err == nil {
				go func() {
					for d := range msgsEvents {
						log.Printf("EVENTS message: %s\n", d.Body)
					}
				}()
				log.Printf(" [*] Waiting for events messages")
			} else {
				log.Printf("Failed to register a consumer for events: %s\n", amqpQueueEvents)
			}

		} else {
			log.Printf("Failed to declare a queue for events %s\n", amqpQueueEvents)
		}
	}
	if len(amqpQueueEventsAdmin) > 0 {
		qEventsAdmin, err := ch.QueueDeclare(
			amqpQueueEventsAdmin, // name
			false,                // durable
			false,                // delete when unused
			false,                // exclusive
			false,                // no-wait
			nil,                  // arguments
		)
		if err == nil {
			msgsEventsAdmin, err := ch.Consume(
				qEventsAdmin.Name, // queue
				"",                // consumer
				true,              // auto-ack
				false,             // exclusive
				false,             // no-local
				false,             // no-wait
				nil,               // args
			)
			if err == nil {
				go func() {
					for d := range msgsEventsAdmin {
						log.Printf("EVENTSADMIN message: %s\n", d.Body)
					}
				}()
				log.Printf(" [*] Waiting for events admin messages")
			} else {
				log.Printf("Failed to register a consumer for events admin: %s\n", amqpQueueEventsAdmin)
			}
		} else {
			log.Printf("Failed to declare a queue for events admin: %s\n", amqpQueueEventsAdmin)
		}
	}

	<-forever

}
