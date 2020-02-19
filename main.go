package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stephen-gardner/intra"
)

type Config struct {
	ListenDomain string `json:"listenDomain"`
	ListenPort   int    `json:"listenPort"`
	Database     struct {
		Address  string `json:"address"`
		User     string `json:"user"`
		Password string `json:"password"`
		Name     string `json:"name"`
	} `json:"database"`
	CampusDomain      string `json:"campusDomain"`
	BatmanEndpoint    string `json:"batmanEndpoint"`
	BatmanMaxAttempts int    `json:"batmanMaxAttempts"`
	Vogsphere         struct {
		Address        string `json:"address"`
		Port           int    `json:"port"`
		User           string `json:"user"`
		PrivateKeyPath string `json:"privateKeyPath"`
		Path           string `json:"path"`
	} `json:"vogsphere"`
	Slack struct {
		Channel                string `json:"channel"`
		InteractiveCloseReason string `json:"interactiveCloseReason"`
	} `json:"slack"`
}

var config Config

func init() {
	intra.SetCacheTimeout(120)
}

func outputErr(err error, fatal bool) {
	log.Println(err)
	sentry.CaptureException(err)
	sentry.Flush(5 * time.Second)
	if fatal {
		os.Exit(1)
	}
}

func loadConfig(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	return nil
}

func main() {
	if err := loadConfig("config.json"); err != nil {
		outputErr(err, true)
	}
	if err := openDatabaseConnection(); err != nil {
		outputErr(err, true)
	}
	rq := &reportQueue{
		in:  make(chan *teamReport),
		out: make(chan *teamReport),
	}
	iq := make(interactQueue)
	go rq.processInput()
	go rq.processOutput()
	go iq.processInput()
	listen(rq, iq)
}
