package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type EventRequest struct {
	Group     string `json:"group"`
	Stream    string `json:"stream"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type Event struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

func getDB() *sql.DB {
	db, err := sql.Open("sqlite3", "./logger.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS events(\"group\" TEXT, stream TEXT, timestamp TEXT, message TEXT)")
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	return db
}

var db *sql.DB

func main() {
	db = getDB()
	http.HandleFunc("/log", handleMessage)
	println("Starting server at http://localhost:4964")
	http.ListenAndServe(":4964", nil)
	defer db.Close()
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		group := r.URL.Query().Get("group")
		stream := r.URL.Query().Get("stream")
		if group == "" || stream == "" {
			http.Error(w, "Invalid Request", http.StatusBadRequest)
		} else {
			rows, err := db.Query("SELECT timestamp, message FROM events WHERE \"group\" = ? AND stream = ? ORDER BY timestamp", group, stream)
			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}
			defer rows.Close()
			var messages []Event
			for rows.Next() {
				var msg Event
				err = rows.Scan(&msg.Timestamp, &msg.Message)
				if err != nil {
					log.Fatalf("Failed to scan row: %v", err)
				}
				messages = append(messages, msg)
			}
			if err = rows.Err(); err != nil {
				log.Fatalf("Failed to iterate rows: %v", err)
			}
			enc := json.NewEncoder(w)
			if err := enc.Encode(messages); err != nil {
				log.Fatalf("Failed to encode response: %v", err)
			}
		}
	} else if r.Method == http.MethodPost {
		dec := json.NewDecoder(r.Body)
		var msg EventRequest
		if err := dec.Decode(&msg); err != nil {
			http.Error(w, "Invalid Request", http.StatusBadRequest)
		} else {
			stmt, err := db.Prepare("INSERT INTO events(\"group\", stream, timestamp, message) values(?,?,?,?)")
			if err != nil {
				log.Fatalf("Failed to prepare statement: %v", err)
			}
			_, err = stmt.Exec(msg.Group, msg.Stream, msg.Timestamp, msg.Message)
			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}
		}
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}
