// Database layer.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"syscall"

	_ "github.com/lib/pq"
)

// CIDB is the database api for ci system.
type CIDB struct {
	DB *sql.DB
}

// PushEvent that ci server actually used.
type PushEvent struct {
	Ref      string
	Head     string
	CloneURL string
}

// openCIDB opens database.
func openCIDB(username string, passwd string, database string) (db *CIDB, err error) {
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
func (db *CIDB) AddPushEvent(event PushEvent) (buildID int64, err error) {
	addPushEventStmt, err := db.DB.Prepare("select new_push_event($1, $2, $3)")
	if err != nil {
		return
	}

	err = addPushEventStmt.QueryRow(event.Head, event.Ref, event.CloneURL).Scan(&buildID)
	if err != nil {
		return
	}

	err = addPushEventStmt.Close()
	return
}

// removePushEvent Only used for unittest.
func (db *CIDB) removePushEvent(event PushEvent, buildID int64) (err error) {
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
func (db *CIDB) GetPushEventByBuildID(buildID int64) (pushEvent PushEvent, err error) {
	pushEvent = PushEvent{}
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
func (db *CIDB) AppendBuildOutput(buildID int64, line string, stdout bool) (err error) {
	if line == "" {
		return
	}

	var channelStr string
	if stdout {
		channelStr = "stdout"
	} else {
		channelStr = "stderr"
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
func (db *CIDB) GetBuildOutputSince(buildID, lineno, limit int64) (output []CommandLineOutput, err error) {
	log.Println(buildID, lineno, limit)
	var rows *sql.Rows
	if limit == -1 {
		rows, err = db.DB.Query("SELECT unnest(outputs[$1:array_length(outputs, 1)]), "+
			"unnest(outputChannels[$1:array_length(outputChannels, 1)]) FROM Builds WHERE id = $2",
			lineno, buildID)
	} else {
		rows, err = db.DB.Query("SELECT unnest(outputs[$1:$3]), "+
			"unnest(outputChannels[$1:$3]) FROM Builds WHERE id = $2",
			lineno, buildID, lineno+limit)
	}
	if err != nil {
		return
	}
	var channelTmp string
	for rows.Next() {
		opt := CommandLineOutput{}
		err = rows.Scan(&opt.Content, &channelTmp)
		if err != nil {
			return
		}
		if channelTmp == "stdout" {
			opt.Channel = syscall.Stdout
		} else {
			opt.Channel = syscall.Stderr
		}
		output = append(output, opt)
	}
	return
}

// GetBuildStatus get build status by build id
func (db *CIDB) GetBuildStatus(buildID int64) (status BuildStatus, err error) {
	var statusStr string
	err = db.DB.QueryRow("SELECT status FROM Builds WHERE id = $1 LIMIT 1", buildID).Scan(&statusStr)
	status = BuildStatus(statusStr)
	return
}

// GetBuildOutput get the whole build output
func (db *CIDB) GetBuildOutput(buildID int64) (output []CommandLineOutput, err error) {
	output, err = db.GetBuildOutputSince(buildID, 0, -1)
	return
}

// BuildStatus in database
type BuildStatus string

const (
	// BuildRunning Build Status in Database, running.
	BuildRunning = "running"
	// BuildSuccess Build Status in Database, success.
	BuildSuccess = "success"
	// BuildError Build Status in Database, error. Error means some build system internal error happend and the ci script not run.
	BuildError = "error"
	// BuildFailed Build Status in Database, failed.
	BuildFailed = "failed"
	// BuildQueued Build Status in Database, queued.
	BuildQueued = "queued"
)

// UpdateBuildStatus update build status in database.
func (db *CIDB) UpdateBuildStatus(buildID int64, status BuildStatus) (err error) {
	_, err = db.DB.Exec("UPDATE Builds SET status = $1 WHERE id = $2", string(status), buildID)
	return
}

// VersionWithStatus git commit with build status
type VersionWithStatus struct {
	Sha    string
	Status BuildStatus
}

// ListRecordPushByBranchName list all push event in database, by branch name.
func (db *CIDB) ListRecordPushByBranchName(branchName string, limit, offset int) (builds []VersionWithStatus, err error) {
	ref := fmt.Sprintf("refs/heads/%s", branchName)
	stmt := `SELECT tmp.ph, tmp.bs
FROM (
SELECT ROW_NUMBER() OVER (PARTITION BY pb.push_head ORDER BY b.id) as r, b.status as bs, pb.push_head as ph FROM PushBuilds AS pb INNER JOIN Builds AS b ON b.id = pb.build_id WHERE 
	pb.push_head in (SELECT pe.head FROM PushEvents AS pe WHERE pe.ref = $1 ORDER BY pe.createTime DESC)
	) as tmp 
WHERE tmp.r <= 1
LIMIT $2
OFFSET $3
`
	rows, err := db.DB.Query(stmt, ref, limit, offset)
	if err != nil {
		return
	}
	var sha string
	var status string
	for rows.Next() {
		err = rows.Scan(&sha, &status)
		if err != nil {
			return
		}
		bs := VersionWithStatus{sha, BuildStatus(status)}
		builds = append(builds, bs)
	}

	return
}

// GetPushEventByHead return the push event object by head sha1
func (db *CIDB) GetPushEventByHead(sha string) (event PushEvent, err error) {
	stmt := `SELECT head, ref, clone_url FROM PushEvents WHERE head = $1 LIMIT 1`
	event = PushEvent{}
	err = db.DB.QueryRow(stmt, sha).Scan(&event.Head, &event.Ref, &event.CloneURL)
	return
}

// GetBuildIDFromPushEventHead return all build id associate by push event
func (db *CIDB) GetBuildIDFromPushEventHead(sha string) (buildID []int64, err error) {
	stmt := `SELECT build_id FROM PushBuilds WHERE push_head = $1 ORDER BY build_id DESC`
	rows, err := db.DB.Query(stmt, sha)
	if err != nil {
		return
	}
	var tmp int64
	for rows.Next() {
		err = rows.Scan(&tmp)
		if err != nil {
			return
		}
		buildID = append(buildID, tmp)
	}
	return
}
