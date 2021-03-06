package main

import (
	_ "net/http/pprof"
	"os"

	"math/rand"
	"time"

	"io/ioutil"
	"net/http"

	"os/signal"

	"fmt"

	"github.com/Financial-Times/message-queue-go-producer/producer"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"github.com/kr/pretty"
)

const messageTimestampDateFormat = "2006-01-02T15:04:05.000Z"

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
var messageProducer producer.MessageProducer

func main() {
	app := cli.App("up-queue-sender", "Consumes a JSON payload, and sends it to a queue.")
	addr := app.String(cli.StringOpt{
		Name:   "destination-address",
		Value:  "http://localhost:8080",
		Desc:   "Address used by the producer to connect to the queue",
		EnvVar: "ADDR",
	})
	topic := app.String(cli.StringOpt{
		Name:   "destination-topic",
		Value:  "NativeCmsMetadataPublicationEvents",
		Desc:   "The topic to write the V1 metadata to",
		EnvVar: "TOPIC",
	})
	queue := app.String(cli.StringOpt{
		Name:   "destination-queue",
		Value:  "kafka",
		Desc:   "The queue used by the republisher",
		EnvVar: "QUEUE",
	})
	app.Action = func() {
		messageProducer = producer.NewMessageProducer(producer.MessageProducerConfig{Addr: *addr, Topic: *topic, Queue: *queue})
		log.Infof("[Startup] Using producer: %# v \n.", pretty.Formatter(messageProducer))
		serve()
	}
	app.Run(os.Args)
}

func serve() {
	m := mux.NewRouter()
	http.Handle("/", handlers.CombinedLoggingHandler(os.Stdout, m))
	m.HandleFunc("/message/{id}", handleRequest).Methods("PUT")

	go func() {
		log.Infof("Listening on [%d].\n", 8080)
		err := http.ListenAndServe(fmt.Sprintf(":%d", 8080), nil)
		if err != nil {
			log.Printf("Web server failed: [%v].\n", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// wait for ctrl-c
	<-c
	println("Exiting.")
}

func buildHeader(uuid string) map[string]string {
	return map[string]string{
		"Message-Id":        uuid,
		"Message-Type":      "cms-content-metadata-published",
		"Content-Type":      "application/json",
		"X-Request-Id":      generateTID(),
		"Origin-System-Id":  "http://cmdb.ft.com/systems/methode-web-pub",
		"Message-Timestamp": time.Now().Format(messageTimestampDateFormat),
	}
}

func generateTID() string {
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return "tid_" + string(b)
}

func init() {
	log.SetFormatter(new(log.JSONFormatter))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	id := mux.Vars(r)["id"]
	message := producer.Message{Headers: buildHeader(id), Body: string(payload)}
	err = messageProducer.SendMessage(id, message)
	if err != nil {
		log.Errorf("Error sending concept suggestion to queue: [%v]", err.Error())
	}
}
