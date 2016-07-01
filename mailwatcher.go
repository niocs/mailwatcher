package main

import (
	"regexp"
	"os"
        "fmt"
	"log"
	"time"
	"io/ioutil"
	"database/sql"
	"github.com/niocs/sflag"
	"github.com/niocs/ezGmail"
	_ "github.com/mattn/go-sqlite3"
)

func extractEmail(addr string) string {
	re := regexp.MustCompile(`([a-zA-Z0-9._%+\-]+)@([a-zA-Z0-9.\-]+)\.([a-zA-Z]{2,5})`)
	return re.FindString(addr)
}

func loadSqlite(dbpath string) (db *sql.DB) {
	if _, err := os.Stat(dbpath); os.IsNotExist(err) {
		db, err = sql.Open("sqlite3", dbpath)
		if err != nil {
			log.Fatal(err)
		}
		createStmt := `
			CREATE TABLE mailidx(
				messageid   CHAR(20) PRIMARY KEY NOT NULL,
				threadid    CHAR(20)             NOT NULL,
				date        CHAR(9)              NOT NULL,
				time        CHAR(10)             NOT NULL,
				sender      TEXT                 NOT NULL,
				subject     TEXT                 NOT NULL,
				filename    TEXT                 NOT NULL,
				attachments TEXT
			)`
		indexStmt := `CREATE INDEX threadidx ON mailidx(threadid)`
		_, err = db.Exec(createStmt)
		if err != nil {
			log.Fatal(err)
		}
		_, err = db.Exec(indexStmt)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		db, err = sql.Open("sqlite3", dbpath)
		if err != nil {
			log.Fatal(err)
		}
	}
	return db
}

var opt = struct {
	Usage       string    "Prints usage string"
	StartDate   string  "Find emails starting from this date. Default to oldest"
	EndDate     string  "Find emails upto this date. Default to latest"
	NewerThanN  string     "Find emails newer than N days"
	OlderThanN  string     "Find emails older than N days"
	MaxResults  int64   "Max emails to find. Can be up 500. Defaults to 500|500"
	Basedir     string  "Basedir to download emails into. If doesn't exist, will be created"}{}

func PrintUsage(errcode int) {
	fmt.Println(`
Usage: mailwatcher  --Basedir      <basedir>    #Download to this dir                                                                                                                                                                          
                   [--StartDate    <YYYYMMDD>]  #find emails from this date.                                                                                                                                                                   
                   [--EndDate      <YYYYMMDD>]  #find emails to this date.                                                                                                                                                                     
                   [--NewerThanN   <N>]         #find emails newer than N days.                                                                                                                                                                
                   [--OlderThanN   <N>]         #find emails older than N days.                                                                                                                                                                
                   [--MaxResults   <count>]     #find "count" emails in this run.                                                                                                                                                              
Use either --StartDate/--EndDate  or  --NewerThanN/--OlderThanN                                                                                                                                                                                
`)
	os.Exit(errcode)
}

func validateArgs() {
	if opt.Basedir == "" {
		PrintUsage(1)
	}
}

func main() {
	sflag.Parse(&opt)
	validateArgs()
	var gs ezGmail.GmailService
	// InitSrv() uses client_secret.json to try to get a OAuth 2.0 token,  , if not present already.
	
	gs.InitSrv()
	db := loadSqlite(opt.Basedir + "/index.sqlite.db")
	defer db.Close()
	// We compose a search statement with filter functions
	gs.InInbox()
	if len(opt.NewerThanN) > 0 {
		gs.NewerThanRel(opt.NewerThanN + "d")
	}
	if len(opt.OlderThanN) > 0 {
		gs.OlderThanRel(opt.OlderThanN + "d")
	}
	if len(opt.StartDate) > 0 {
		timeObj, err := time.Parse("20060102",opt.StartDate)
		if err != nil {
			log.Fatal(err)
		}
		gs.NewerThan(timeObj.Format("2006/01/02"))
	}
	if len(opt.EndDate) > 0 {
		timeObj, err := time.Parse("20060102",opt.EndDate)
		if err != nil {
			log.Fatal(err)
		}
		gs.OlderThan(timeObj.Format("2006/01/02"))
	}
	gs.MaxResults(opt.MaxResults)
	

	// GetMessages() tries to execute the search statement and get a list of messages
	for _, ii := range(gs.GetMessages()) {
		var tmp string
		err := db.QueryRow("SELECT messageid FROM mailidx WHERE messageid = ?",ii.GetMessageId()).Scan(&tmp)
		switch {
		case err == nil:
			continue
		case err != sql.ErrNoRows:
			log.Fatal(err)
		}
		
		//threadid    := ii.GetThreadId()
		from        := extractEmail(ii.GetFrom())
		date        := ii.GetDate().Format("20060102")
		timestamp   := ii.GetDate().Format("20060102-150405")
		
		dir := opt.Basedir + "/" + from + "/" + date
		os.MkdirAll(dir, 0755)
		var fp *os.File
		var fname string
		var attNames string
		var ctr = 0
		for {
			fname    = fmt.Sprintf("%s/%s.%03d", dir, timestamp, ctr)
			fp, err = os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
			if err == nil {
				break
			}
			ctr += 1
			if ctr > 999 {
				log.Fatal("Can't create file: " + fname + " : " + err.Error() + "\n")
			}
		}
		if ii.HasBodyText() {
			fmt.Fprint(fp, string(ii.GetBodyText()))
		} else if ii.HasBodyHtml() {
			fmt.Fprint(fp, string(ii.GetBodyHtml()))
		}
		fp.Close()
		if ii.HasAttachments() {
			attDir := fname + ".d"
			os.MkdirAll(attDir, 0755)
			for _, att := range(ii.GetAttachments()) {
				if len(attNames) > 0 { attNames += ";" }
				attNames += att.GetFilename()
				ioutil.WriteFile(attDir + "/" + att.GetFilename(), att.GetData(), 0755)
			}
		}
		_,err = db.Exec("INSERT INTO mailidx VALUES (?,?,?,?,?,?,?,?)",ii.GetMessageId(), ii.GetThreadId(), ii.GetDate().Format("20060102"), ii.GetDate().Format("150405"),ii.GetFrom(),ii.GetSubject(),fname,attNames)
		if err != nil {
			log.Fatal(err)
		}
		
	}
}
