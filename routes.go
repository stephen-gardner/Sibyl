package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/stephen-gardner/intra"
)

func (queue *reportQueue) handleTeamMarked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	if r.Header.Get("X-Secret") != os.Getenv("X_SECRET") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	deliveryID := r.Header.Get("X-Delivery")
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		outputErr(err, false)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Printf("<%s> IN:\n%s\n", deliveryID, string(data))
	team := &intra.WebTeam{}
	if err := json.Unmarshal(data, &team); err != nil {
		err = fmt.Errorf("[400] %s: %s", err.Error(), string(data))
		outputErr(err, false)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	report := &teamReport{}
	if err := report.loadData(r.Context(), deliveryID, team); err != nil {
		outputErr(err, false)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	go func(queue *reportQueue, report *teamReport) {
		queue.in <- report
	}(queue, report)
	w.WriteHeader(http.StatusOK)
}

func handleSlackInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	if err := r.ParseForm(); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	payload := &SlackInteraction{}
	data := []byte(r.Form.Get("payload"))
	if err := json.Unmarshal(data, payload); err != nil {
		err = fmt.Errorf("[400] %s: %s", err.Error(), string(data))
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payload.Type != "block_actions" || len(payload.Actions) == 0 || payload.Actions[0].ActionID != "manage_report" {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	go func(payload *SlackInteraction) {
		if err := payload.process(); err != nil {
			log.Println(err)
		}
	}(payload)
	w.WriteHeader(http.StatusOK)
}

func listen(tq *reportQueue) {
	http.HandleFunc("/sibyl/slack", handleSlackInteraction)
	http.HandleFunc("/teams/marked", tq.handleTeamMarked)
	// Display picture for anonymized accounts
	http.HandleFunc("/3b3.jpg", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "images/3b3.jpg")
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.ListenPort), nil); err != nil {
		outputErr(err, true)
	}
}
