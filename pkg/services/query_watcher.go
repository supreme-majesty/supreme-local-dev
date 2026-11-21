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

// QueryWatcher shows the queries run on SLD's MySQL/MariaDB server on the
// X-Ray page. It reads the server's general log, which records every query
// and costs performance and disk, so the log is on only while someone has the
// page open: the page calls Touch periodically, and after watchIdle without a
// call the log is turned off and emptied.
type QueryWatcher struct {
	db        *sql.DB
	bus       *events.Bus
	lastTime  time.Time
	running   bool
	lastTouch time.Time
	mu        sync.Mutex
	stopCh    chan struct{}
}

const watchIdle = 60 * time.Second

func NewQueryWatcher(bus *events.Bus, dsn string) (*QueryWatcher, error) {
	// parseTime: general_log.event_time is scanned into time.Time.
	if strings.Contains(dsn, "?") {
		dsn += "&parseTime=true"
	} else {
		dsn += "?parseTime=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	return &QueryWatcher{db: db, bus: bus}, nil
}

// Touch keeps the watcher running, starting it if needed.
func (w *QueryWatcher) Touch() error {
	w.mu.Lock()
	w.lastTouch = time.Now()
	running := w.running
	w.mu.Unlock()
	if running {
		return nil
	}
	return w.Start()
}

func (w *QueryWatcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil
	}
	if _, err := w.db.Exec("SET GLOBAL log_output = 'TABLE'"); err != nil {
		return fmt.Errorf("failed to set log_output: %w", err)
	}
	if _, err := w.db.Exec("SET GLOBAL general_log = 'ON'"); err != nil {
		return fmt.Errorf("failed to set general_log: %w", err)
	}
	var now time.Time
	if err := w.db.QueryRow("SELECT NOW(6)").Scan(&now); err != nil {
		now = time.Now()
	}
	w.lastTime = now
	if w.lastTouch.IsZero() {
		w.lastTouch = time.Now()
	}
	w.running = true
	w.stopCh = make(chan struct{})
	go w.poll(w.stopCh)
	return nil
}

// Stop turns the general log off and empties it.
func (w *QueryWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		w.running = false
		close(w.stopCh)
	}
	w.disableLocked()
}

// Reset turns off and empties a general log left on (by an earlier SLD that
// kept it on all the time).
func (w *QueryWatcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	var on int
	if err := w.db.QueryRow("SELECT @@GLOBAL.general_log").Scan(&on); err == nil && on == 1 {
		w.disableLocked()
	}
}

func (w *QueryWatcher) disableLocked() {
	_, _ = w.db.Exec("SET GLOBAL general_log = 'OFF'")
	_, _ = w.db.Exec("TRUNCATE TABLE mysql.general_log")
}

func (w *QueryWatcher) poll(stop chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			idle := time.Since(w.lastTouch) > watchIdle
			w.mu.Unlock()
			if idle {
				w.Stop()
				return
			}
			w.fetchQueries()
		case <-stop:
			return
		}
	}
}

func (w *QueryWatcher) fetchQueries() {
	rows, err := w.db.Query(`
		SELECT event_time, user_host, thread_id, server_id, command_type, CONVERT(argument USING utf8mb4)
		FROM mysql.general_log
		WHERE event_time > ? AND command_type = 'Query'
		ORDER BY event_time ASC
		LIMIT 500`, w.lastTime)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var eventTime time.Time
		var q QueryLog
		var arg []byte // argument is a mediumblob
		if err := rows.Scan(&eventTime, &q.UserHost, &q.ThreadID, &q.ServerID, &q.Command, &arg); err != nil {
			continue
		}
		w.lastTime = eventTime

		q.EventTime = eventTime.Format(time.RFC3339Nano)
		q.Argument = string(arg)
		q.ID = fmt.Sprintf("query-%d-%d", q.ThreadID, eventTime.UnixNano())

		// Skip SLD's own housekeeping and other noise.
		lower := strings.ToLower(q.Argument)
		if strings.Contains(lower, "mysql.general_log") ||
			strings.Contains(lower, "set global") ||
			strings.HasPrefix(lower, "show ") ||
			strings.HasPrefix(lower, "select now(") ||
			strings.HasPrefix(lower, "select @@") ||
			q.Argument == "COMMIT" {
			continue
		}
		w.bus.Publish(events.Event{Type: events.DatabaseQuery, Payload: q})
	}
}
