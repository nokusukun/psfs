package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	messages "github.com/nokusukun/psfs/pb"
	"github.com/nokusukun/psfs/satellite"
)

type WriteRequest struct {
	PacketType  string      `json:"type"`
	Destination string      `json:"destination"`
	Namespace   string      `json:"namespace"`
	Content     interface{} `json:"content"`
}

func generateAPI(sat *satellite.Satellite) *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Retieving Peers")
		var ids []string

		for _, p := range sat.Peers {
			ids = append(ids, p.ID.Pretty())
		}

		_ = json.NewEncoder(w).Encode(ids)
	}).Methods("GET")

	router.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		request := WriteRequest{}

		_ = json.NewDecoder(r.Body).Decode(&request)

		var errCode string
		p, exists := sat.Peers[request.Destination]
		if exists {
			err := p.Write(messages.PacketType(messages.PacketType_value[request.PacketType]),
				request.Namespace,
				request.Content)
			if err != nil {
				errCode = fmt.Sprintf("failed to write: %v", err)
			}
		} else {
			errCode = fmt.Sprintf("peer does not exist: %v", request.Destination)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": errCode,
		})
	})

	router.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		request := WriteRequest{}

		_ = json.NewDecoder(r.Body).Decode(&request)

		var errCode string

		err := sat.BroadcastMessage(request.Namespace, request.Content)
		if err != nil {
			errCode = fmt.Sprintf("failed to broadcast: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": errCode,
		})
	})

	router.HandleFunc("/whois/{peerId}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		errCode := ""
		var response interface{}

		p, exists := sat.Peers[vars["peerId"]]
		if exists {
			resp, err := p.Request("whois", "")
			if err != nil {
				errCode = fmt.Sprintf("failed request: %v", err)
			} else {
				fmt.Println("Waiting for response...")
				response = <-resp
			}
		} else {
			errCode = fmt.Sprintf("peer does not exist: %v", vars["peerId"])
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":    errCode,
			"response": response,
		})

	}).Methods("GET")

	return router
}
