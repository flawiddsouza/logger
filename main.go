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
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS group_index ON events(\"group\")")
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS stream_index ON events(stream)")
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
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
		if group == "" {
			// get all groups with last event time
			rows, err := db.Query("SELECT \"group\", MAX(timestamp) AS lastEventTime FROM events GROUP BY \"group\" ORDER BY lastEventTime DESC")
			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}
			defer rows.Close()
			var groups []map[string]string
			for rows.Next() {
				var group string
				var lastEventTime string
				err = rows.Scan(&group, &lastEventTime)
				if err != nil {
					log.Fatalf("Failed to scan row: %v", err)
				}
				groups = append(groups, map[string]string{"group": group, "lastEventTime": lastEventTime})
			}
			if err = rows.Err(); err != nil {
				log.Fatalf("Failed to iterate rows: %v", err)
			}
			enc := json.NewEncoder(w)
			if err := enc.Encode(groups); err != nil {
				log.Fatalf("Failed to encode response: %v", err)
			}
		} else if group != "" && stream == "" {
			// get all streams for group with last event time
			rows, err := db.Query("SELECT stream, MAX(timestamp) AS lastEventTime FROM events WHERE \"group\" = ? GROUP BY stream ORDER BY lastEventTime DESC", group)
			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}
			defer rows.Close()
			var streams []map[string]string
			for rows.Next() {
				var stream string
				var lastEventTime string
				err = rows.Scan(&stream, &lastEventTime)
				if err != nil {
					log.Fatalf("Failed to scan row: %v", err)
				}
				streams = append(streams, map[string]string{"stream": stream, "lastEventTime": lastEventTime})
			}
			if err = rows.Err(); err != nil {
				log.Fatalf("Failed to iterate rows: %v", err)
			}
			enc := json.NewEncoder(w)
			if err := enc.Encode(streams); err != nil {
				log.Fatalf("Failed to encode response: %v", err)
			}
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
