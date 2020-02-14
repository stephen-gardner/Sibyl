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
	ListenDomain           string
	ListenPort             int
	CampusDomain           string
	BatmanEndpoint         string
	RepoAddress            string
	RepoPort               int
	RepoUser               string
	RepoPrivateKeyPath     string
	RepoPath               string
	SlackReportChannel     string
	InteractiveCloseReason string
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
	tq := &reportQueue{
		in:  make(chan *teamReport),
		out: make(chan *teamReport),
	}
	go tq.processInput()
	go tq.processOutput()
	listen(tq)
}
