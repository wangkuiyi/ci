// Database layer.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"syscall"

	_ "github.com/lib/pq"
)

// CIDB is the database api for ci system.
type CIDB struct {
	DB *sql.DB
}

// Create a database.
func newCIDB(username string, passwd string, database string) (db *CIDB, err error) {
	sdb, err := sql.Open("postgres", fmt.Sprintf("postgres://%s:%s@localhost/%s?sslmode=disable",
		username, passwd, database))
	if err != nil {
		return
	}
	err = sdb.Ping()
	if err != nil {
		return
	}
	db = &CIDB{sdb}
	return
}

// Close the database.
func (db *CIDB) Close() {
	db.DB.Close()
}

// AddPushEvent insert a PushEvent into database.
func (db *CIDB) AddPushEvent(event *PushEvent) (buildID int64, err error) {
	addPushEventStmt, err := db.DB.Prepare("select new_push_event($1, $2, $3)")
	if err != nil {
		return
	}
	defer addPushEventStmt.Close()
	err = addPushEventStmt.QueryRow(event.Head, event.Ref, event.CloneURL).Scan(&buildID)
	return
}

// removePushEvent Only used for unittest.
func (db *CIDB) removePushEvent(event *PushEvent, buildID int64) (err error) {
	_, err = db.DB.Exec("DELETE FROM PushBuilds WHERE push_head = $1", event.Head)
	if err != nil {
		return
	}
	_, err = db.DB.Exec("DELETE FROM PushEvents WHERE head = $1", event.Head)
	if err != nil {
		return
	}
	_, err = db.DB.Exec("DELETE FROM Builds WHERE id = $1", buildID)
	return
}

// GetPushEventByBuildID Get PushEvent From Database
func (db *CIDB) GetPushEventByBuildID(buildID int64) (pushEvent *PushEvent, err error) {
	pushEvent = &PushEvent{}
	err = db.DB.QueryRow("SELECT pe.head, pe.ref, pe.clone_url FROM PushEvents AS pe JOIN PushBuilds as pb"+
		" ON pe.head = pb.push_head WHERE pb.build_id = $1 LIMIT 1", buildID).Scan(
		&pushEvent.Head, &pushEvent.Ref, &pushEvent.CloneURL)
	return
}

// RecoverFromPreviousDown in case of the previous ci server was killed.
func (db *CIDB) RecoverFromPreviousDown(buildChan chan int64) (err error) {
	r, err := db.DB.Exec("UPDATE Builds SET status='queued', outputs=array[]::text[]," +
		" outputChannels=array[]::OutputChannel[] " +
		"WHERE status='running'")
	if err != nil {
		return
	}

	rowsCount, err := r.RowsAffected()
	if err != nil {
		return
	}
	log.Printf("Reset previous running job to queued, count = %d\n", rowsCount)

	rows, err := db.DB.Query("SELECT id FROM Builds WHERE status='queued'")
	if err != nil {
		return
	}
	var bid int64
	for rows.Next() {
		err = rows.Scan(&bid)
		if err != nil {
			return
		}
		buildChan <- bid
	}
	return
}

// AppendBuildOutput append build output into database
func (db *CIDB) AppendBuildOutput(buildID int64, line string, channel int) (err error) {
	var channelStr string
	switch channel {
	case syscall.Stderr:
		channelStr = "stderr"
	case syscall.Stdout:
		channelStr = "stdout"
	default:
		err = errors.New("Unsupport output channel, should be stdout or stderr")
		return
	}

	_, err = db.DB.Exec("UPDATE Builds SET outputs = array_append(outputs, $1), "+
		"outputChannels=array_append(outputChannels,$2) WHERE id = $3",
		line, channelStr, buildID)
	return
}

// CommandLineOutput the shell output content and channel (stdout/stderr)
type CommandLineOutput struct {
	// Content
	Content string
	Channel int
}

// GetBuildOutputSince Get build output from line number `lineno`, the total size is `limit`.
// If `limit = -1`, means there is no limit
func (db *CIDB) GetBuildOutputSince(buildID int64, lineno int, limit int) (output []CommandLineOutput, err error) {
	var totalLength int
	err = db.DB.QueryRow("SELECT array_length(outputs, 1) FROM Builds WHERE id = $1", buildID).Scan(&totalLength)
	if err != nil {
		return
	}

	// calculate the output length, which return.
	length := totalLength - lineno
	if length <= 0 {
		err = errors.New("Line no > total length, database error")
		return
	}
	if limit >= 0 {
		if limit < length {
			length = limit
		}
	}

	var rows *sql.Rows
	if limit == -1 {
		rows, err = db.DB.Query("SELECT unnest(outputs[$1:array_length(outputs, 1)]), "+
			"unnest(outputChannels[$1:array_length(outputChannels, 1)]) FROM Builds WHERE id = $2",
			lineno, buildID)
	} else {
		rows, err = db.DB.Query("SELECT unnest(outputs[$1:$3]), "+
			"unnest(outputChannels[$1:$3]) FROM Builds WHERE id = $2",
			lineno, buildID, lineno+length)
	}
	if err != nil {
		return
	}
	defer rows.Close()
	log.Println(length)
	output = make([]CommandLineOutput, length)
	i := 0
	var channelTmp string
	for rows.Next() {
		err = rows.Scan(&output[i].Content, &channelTmp)
		if err != nil {
			return
		}
		if channelTmp == "stdout" {
			output[i].Channel = syscall.Stdout
		} else {
			output[i].Channel = syscall.Stderr
		}
		i++
	}
	return
}

// GetBuildOutput get the whole build output
func (db *CIDB) GetBuildOutput(buildID int64) (output []CommandLineOutput, err error) {
	output, err = db.GetBuildOutputSince(buildID, 0, -1)
	return
}

const (
	// BuildRunning Build Status in Database, running.
	BuildRunning = "running"
	// BuildSuccess Build Status in Database, success.
	BuildSuccess = "success"
	// BuildError Build Status in Database, error. Error means some build system internal error happend and the ci script not run.
	BuildError = "error"
	// BuildFailed Build Status in Database, failed.
	BuildFailed = "failed"
)

// UpdateBuildStatus update build status in database.
func (db *CIDB) UpdateBuildStatus(buildID int64, status string) (err error) {
	_, err = db.DB.Exec("UPDATE Builds SET status = $1 WHERE id = $2", status, buildID)
	return
}
