package main

import "log"

func main() {
	opts := ParseArgs()
	github := newGithubAPI(opts)
	db, err := newCIDB(opts.Database.User, opts.Database.Password, opts.Database.DatabaseName)
	checkNoErr(err)
	buildChan := make(chan int64, 256)
	go func() { checkNoErr(db.RecoverFromPreviousDown(buildChan)) }()
	builder, err := newBuilder(buildChan, opts, db, github)
	builder.Start()
	defer builder.Close()

	serv := newHTTPServer(opts, db)
	go func() {
		checkNoErr(serv.ListenAndServe())
	}()

	for ev := range serv.EventQueue {
		switch ev.(type) {
		case *PushEvent:
			{
				event := ev.(*PushEvent)
				bid, err := db.AddPushEvent(event) // add event to db
				checkNoErr(err)
				buildChan <- bid
			}
		}
	}
}

func checkNoErr(err error) {
	if err != nil {
		log.Panic(err.Error())
	}
}
