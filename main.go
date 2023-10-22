package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/meilisearch/meilisearch-go"
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

func createTable(db *sql.DB, table string, columns []string) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS " + table + "(" + strings.Join(columns, ", ") + ")")
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
}

func createIndex(db *sql.DB, tableColumn string, index string) {
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS " + index + " ON " + tableColumn)
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
}

func createCompositeUniqueIndex(db *sql.DB, table string, columns []string, index string) {
	_, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS " + index + " ON " + table + "(" + strings.Join(columns, ", ") + ")")
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
}

func getDB() *sql.DB {
	db, err := sql.Open("postgres", "postgres://postgres:password@localhost:5432/logger?sslmode=disable")
	// on why WAL: https://www.golang.dk/articles/go-and-sqlite-in-the-cloud
	//db, err := sql.Open("sqlite3", "./logger.db?_journal=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	createTable(db, "streams", []string{"\"group\" TEXT", "stream TEXT", "lastEventTime TEXT"})
	createCompositeUniqueIndex(db, "streams", []string{"\"group\"", "stream"}, "group_stream_unique_index")
	createTable(db, "events", []string{"\"group\" TEXT", "stream TEXT", "timestamp TEXT", "message TEXT"})
	createIndex(db, "events(\"group\", stream, timestamp)", "group_stream_timestamp_index")

	return db
}

var db *sql.DB
var meilisearchEventsIndex *meilisearch.Index

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	db = getDB()

	meilisearchClient := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   os.Getenv("MEILISEARCH_API_URL"),
		APIKey: os.Getenv("MEILISEARCH_API_KEY"),
	})

	meilisearchEventsIndex = meilisearchClient.Index("events")

	// deleteAllEvents()
	// os.Exit(0)

	result, err := meilisearchEventsIndex.GetSettings()

	if err != nil {
		log.Printf("Failed to get events index settings: %v", err)
	} else {
		if len(result.FilterableAttributes) < 3 || len(result.SortableAttributes) == 0 {
			println("Updating meilisearch events index settings")
			meilisearchEventsIndex.UpdateSettings(&meilisearch.Settings{
				FilterableAttributes: []string{"group", "stream", "timestamp_epoch"},
				SortableAttributes:   []string{"timestamp_epoch"},
			})
		}
	}

	deleteEventsOlderThan(2)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("ui/dist")))
	mux.HandleFunc("/log", handleMessage)
	mux.HandleFunc("/index", handleIndexing)
	println("Starting server at http://localhost:4964")
	http.ListenAndServe(":4964", mux)

	defer db.Close()
}

func createLogger(r *http.Request) func(string, ...interface{}) {
	requestId := uuid.New().String()
	method := r.Method
	path := r.URL.Path

	return func(format string, a ...interface{}) {
		logMessage := fmt.Sprintf(format, a...)
		prefixedMessage := fmt.Sprintf("[%s] %s %s: %s", requestId, method, path, logMessage)
		fmt.Println(prefixedMessage)
	}
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	logger := createLogger(r)

	if r.Method == http.MethodGet {
		group := r.URL.Query().Get("group")
		stream := r.URL.Query().Get("stream")
		search := r.URL.Query().Get("search")
		var streams []map[string]string

		start := time.Now()

		if group == "" {
			// get all groups with last event time
			rows, err := db.Query("SELECT \"group\", MAX(lastEventTime) AS lastEventTime FROM streams GROUP BY \"group\" ORDER BY lastEventTime DESC")
			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}
			defer rows.Close()
			groups := []map[string]string{}
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
			var rows *sql.Rows
			var err error
			if search != "" {
				meilisearchEvents, err := meilisearchEventsIndex.Search(search, &meilisearch.SearchRequest{
					Limit:                 1000, // 1000 is the max limit, as maxTotalHits defaults to 1000 (not sure how to increase that)
					AttributesToHighlight: []string{"message"},
					HighlightPreTag:       "<mark>",
					HighlightPostTag:      "</mark>",
					// Filter: "group = \"" + group + "\"",
				})

				if err != nil {
					log.Fatalf("Failed to execute statement: %v", err)
				}

				streams = []map[string]string{}
				for _, hit := range meilisearchEvents.Hits {
					streams = append(streams, map[string]string{"stream": hit.(map[string]interface{})["stream"].(string), "lastEventTime": hit.(map[string]interface{})["timestamp"].(string), "message": hit.(map[string]interface{})["_formatted"].(map[string]interface{})["message"].(string)})
				}
			} else {
				rows, err = db.Query("SELECT stream, lastEventTime FROM streams WHERE \"group\" = $1 ORDER BY lastEventTime DESC", group)
				if err != nil {
					log.Fatalf("Failed to execute statement: %v", err)
				}
				defer rows.Close()
				streams = []map[string]string{}
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
			}
			enc := json.NewEncoder(w)
			if err := enc.Encode(streams); err != nil {
				log.Fatalf("Failed to encode response: %v", err)
			}
		} else {
			// Start timing DB Query
			queryStart := time.Now()

			rows, err := db.Query("SELECT timestamp, message FROM events WHERE \"group\" = $1 AND stream = $2 ORDER BY timestamp", group, stream)

			logger("DB Query took %v\n", time.Since(queryStart))

			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}

			// Start timing row scan
			rowScanStart := time.Now()

			defer rows.Close()
			messages := []Event{}
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

			logger("Row scan took %v\n", time.Since(rowScanStart))

			enc := json.NewEncoder(w)
			if err := enc.Encode(messages); err != nil {
				log.Fatalf("Failed to encode response: %v", err)
			}
		}

		logger("Total operation took %s\n", time.Since(start))
	} else if r.Method == http.MethodPost {
		start := time.Now()

		dec := json.NewDecoder(r.Body)
		var msg EventRequest
		if err := dec.Decode(&msg); err != nil {
			http.Error(w, "Invalid Request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		stmt, err := db.Prepare("INSERT INTO events(\"group\", stream, timestamp, message) values($1,$2,$3,$4)")
		if err != nil {
			log.Fatalf("Failed to prepare statement: %v", err)
		}
		defer stmt.Close()
		_, err = stmt.Exec(msg.Group, msg.Stream, msg.Timestamp, msg.Message)
		if err != nil {
			log.Fatalf("Failed to execute statement: %v", err)
		}

		stmt, err = db.Prepare("INSERT INTO streams(\"group\", stream, lastEventTime) values($1,$2,$3) ON CONFLICT (\"group\", stream) DO UPDATE SET lastEventTime = $3")
		if err != nil {
			log.Fatalf("Failed to prepare statement: %v", err)
		}
		defer stmt.Close()
		_, err = stmt.Exec(msg.Group, msg.Stream, msg.Timestamp)
		if err != nil {
			log.Fatalf("Failed to execute statement: %v", err)
		}

		timestampEpoch, err := time.Parse(time.RFC3339, msg.Timestamp)

		if err != nil {
			log.Fatalf("Failed to parse timestamp: %v", err)
		}

		meilisearchEventsIndex.AddDocuments([]map[string]interface{}{{
			"id":              msg.Group + "_" + msg.Stream + "_" + strings.ReplaceAll(strings.ReplaceAll(msg.Timestamp, ":", "_"), ".", "_"),
			"group":           msg.Group,
			"stream":          msg.Stream,
			"timestamp":       msg.Timestamp,
			"message":         msg.Message,
			"timestamp_epoch": timestampEpoch.Unix(),
		}})

		w.WriteHeader(http.StatusCreated)

		logger("Total operation took %s\n", time.Since(start))
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func handleIndexing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Start total timing
		start := time.Now()

		const ChunkSize = 500000
		var counter int = 1
		for offset := 0; ; offset += ChunkSize {
			var rows *sql.Rows
			var err error

			// Start timing DB Query
			queryStart := time.Now()

			rows, err = db.Query("SELECT \"group\", stream, timestamp, message FROM events ORDER BY \"group\", stream, timestamp LIMIT $1 OFFSET $2", ChunkSize, offset)

			fmt.Printf("DB Query took %v\n", time.Since(queryStart))

			if err != nil {
				log.Fatalf("Failed to execute statement: %v", err)
			}

			// Start timing row scan
			rowScanStart := time.Now()

			events := []map[string]interface{}{}
			for rows.Next() {
				var group string
				var stream string
				var timestamp string
				var message string
				err = rows.Scan(&group, &stream, &timestamp, &message)
				if err != nil {
					log.Fatalf("Failed to scan row: %v", err)
				}

				timestampEpoch, err := time.Parse(time.RFC3339, timestamp)

				if err != nil {
					log.Fatalf("Failed to parse timestamp: %v", err)
				}

				events = append(events, map[string]interface{}{
					"id":              group + "_" + stream + "_" + strings.ReplaceAll(strings.ReplaceAll(timestamp, ":", "_"), ".", "_"),
					"group":           group,
					"stream":          stream,
					"timestamp":       timestamp,
					"message":         message,
					"timestamp_epoch": timestampEpoch.Unix(),
				})
			}
			if err = rows.Err(); err != nil {
				log.Fatalf("Failed to iterate rows: %v", err)
			}
			rows.Close()

			fmt.Printf("Row scan took %v\n", time.Since(rowScanStart))

			if len(events) == 0 {
				break
			}

			println("Indexing " + strconv.Itoa(len(events)) + " events / " + strconv.Itoa(counter) + " fetch batches done")

			// Start timing meilisearchEventsIndex.AddDocuments
			indexStart := time.Now()

			meilisearchEventsIndex.AddDocumentsInBatches(events, 100000)

			fmt.Printf("Indexing took %v\n", time.Since(indexStart))

			counter++
		}

		fmt.Printf("Total operation took %s\n", time.Since(start))

		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func deleteEventsOlderThan(days int) {
	// Start total timing
	start := time.Now()

	// Start timing DB Query
	queryStart := time.Now()

	stmt, err := db.Prepare("DELETE FROM events WHERE timestamp < $1")
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(time.Now().AddDate(0, 0, -days).Format(time.RFC3339))
	if err != nil {
		log.Fatalf("Failed to execute statement: %v", err)
	}

	fmt.Printf("DB Query event deletion took %v\n", time.Since(queryStart))

	queryStart = time.Now()

	stmt, err = db.Prepare("DELETE FROM streams WHERE lastEventTime < $1")
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(time.Now().AddDate(0, 0, -days).Format(time.RFC3339))
	if err != nil {
		log.Fatalf("Failed to execute statement: %v", err)
	}

	fmt.Printf("DB Query stream deletion took %v\n", time.Since(queryStart))

	// Start timing meilisearchEventsIndex.DeleteDocumentsByFilter
	indexStart := time.Now()

	response, err := meilisearchEventsIndex.DeleteDocumentsByFilter(`timestamp_epoch < "` + strconv.Itoa(int(time.Now().AddDate(0, 0, -days).Unix())) + `"`)

	if err != nil {
		log.Fatalf("Failed to DeleteDocumentsByFilter: %v", err)
	}

	fmt.Printf("Meilisearch response: %v\n", response)

	fmt.Printf("Deleting documents from index took %v\n", time.Since(indexStart))

	fmt.Printf("Total operation took %s\n", time.Since(start))
}

//lint:ignore U1000 This is a utility function
func deleteAllEvents() {
	// Start total timing
	start := time.Now()

	// Start timing DB Query
	queryStart := time.Now()

	stmt, err := db.Prepare("DELETE FROM events")
	if err != nil {
		log.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec()
	if err != nil {
		log.Fatalf("Failed to execute statement: %v", err)
	}

	fmt.Printf("DB Query took %v\n", time.Since(queryStart))

	// Start timing meilisearchEventsIndex.DeleteAllDocuments
	indexStart := time.Now()

	response, err := meilisearchEventsIndex.DeleteAllDocuments()

	if err != nil {
		log.Fatalf("Failed to DeleteAllDocuments: %v", err)
	}

	fmt.Printf("Meilisearch response: %v\n", response)

	fmt.Printf("Deleting all documents from index took %v\n", time.Since(indexStart))

	fmt.Printf("Total operation took %s\n", time.Since(start))
}
