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

type SlackInteraction struct {
	Type string `json:"type"`
	Team struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	} `json:"team"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		TeamID   string `json:"team_id"`
	} `json:"user"`
	APIAppID  string `json:"api_app_id"`
	Container struct {
		Type         string `json:"type"`
		MessageTs    string `json:"message_ts"`
		AttachmentID int    `json:"attachment_id"`
		ChannelID    string `json:"channel_id"`
		IsEphemeral  bool   `json:"is_ephemeral"`
		IsAppUnfurl  bool   `json:"is_app_unfurl"`
	} `json:"container"`
	TriggerID   string `json:"trigger_id"`
	ResponseURL string `json:"response_url"`
	Actions     []struct {
		Type           string `json:"type"`
		BlockID        string `json:"block_id"`
		ActionID       string `json:"action_id"`
		SelectedOption struct {
			Text struct {
				Type  string `json:"type"`
				Text  string `json:"text"`
				Emoji bool   `json:"emoji"`
			} `json:"text"`
			Value string `json:"value"`
		} `json:"selected_option"`
		ActionTs string `json:"action_ts"`
	} `json:"actions"`
}

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
	go func(rq *reportQueue, report *teamReport) {
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
		w.WriteHeader(http.StatusBadRequest)
	}
	payload := &SlackInteraction{}
	if err := json.Unmarshal([]byte(r.Form.Get("payload")), payload); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
	}
	fmt.Printf("%+v\n", payload)
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
