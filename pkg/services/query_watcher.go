package services

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

type QueryLog struct {
	ID        string `json:"id"`
	EventTime string `json:"event_time"`
	UserHost  string `json:"user_host"`
	ThreadID  int64  `json:"thread_id"`
	ServerID  int64  `json:"server_id"`
	Command   string `json:"command"`
	Argument  string `json:"argument"`
}

type QueryWatcher struct {
	db       *sql.DB
	bus      *events.Bus
	lastTime string // MySQL general_log uses timestamp string usually, or time.Time
	running  bool
	mu       sync.Mutex
	stopCh   chan struct{}
}

func NewQueryWatcher(bus *events.Bus, dsn string) (*QueryWatcher, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	return &QueryWatcher{
		db:     db,
		bus:    bus,
		stopCh: make(chan struct{}),
	}, nil
}

func (w *QueryWatcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	// Enable general_log to TABLE
	_, err := w.db.Exec("SET GLOBAL log_output = 'TABLE';")
	if err != nil {
		return fmt.Errorf("failed to set log_output: %w", err)
	}
	_, err = w.db.Exec("SET GLOBAL general_log = 'ON';")
	if err != nil {
		return fmt.Errorf("failed to set general_log: %w", err)
	}

	// Initialize lastTime to now using MySQL's NOW()
	var now string
	err = w.db.QueryRow("SELECT NOW()").Scan(&now)
	if err != nil {
		now = time.Now().Format("2006-01-02 15:04:05")
	}
	w.lastTime = now

	go w.poll()
	return nil
}

func (w *QueryWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)

	// Turn off general log to save resources
	w.db.Exec("SET GLOBAL general_log = 'OFF';")
}

func (w *QueryWatcher) poll() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.fetchQueries()
		case <-w.stopCh:
			return
		}
	}
}

func (w *QueryWatcher) fetchQueries() {
	// general_log table schema:
	// event_time, user_host, thread_id, server_id, command_type, argument
	query := `
		SELECT event_time, user_host, thread_id, server_id, command_type, CONVERT(argument USING utf8) 
		FROM mysql.general_log 
		WHERE event_time > ? AND command_type = 'Query'
		ORDER BY event_time ASC
	`
	rows, err := w.db.Query(query, w.lastTime)
	if err != nil {
		return
	}
	defer rows.Close()

	var lastEventTime time.Time

	for rows.Next() {
		var eventTime time.Time
		var q QueryLog
		var arg []byte // Argument is mediumblob

		if err := rows.Scan(&eventTime, &q.UserHost, &q.ThreadID, &q.ServerID, &q.Command, &arg); err != nil {
			continue
		}

		q.EventTime = eventTime.Format(time.RFC3339)
		q.Argument = string(arg)
		q.ID = fmt.Sprintf("query-%d-%d", q.ThreadID, eventTime.UnixNano())

		// Filter out noisy queries
		lowerArg := strings.ToLower(q.Argument)
		if strings.Contains(lowerArg, "select * from mysql.general_log") ||
			strings.Contains(lowerArg, "set global") ||
			strings.Contains(lowerArg, "show ") ||
			q.Argument == "SELECT NOW()" ||
			q.Argument == "COMMIT" {
			continue
		}

		w.bus.Publish(events.Event{
			Type:    events.DatabaseQuery,
			Payload: q,
		})

		lastEventTime = eventTime
	}

	if !lastEventTime.IsZero() {
		w.lastTime = lastEventTime.Format("2006-01-02 15:04:05.999999")
	}
}
