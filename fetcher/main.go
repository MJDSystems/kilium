package main

import (
	"log"
	"time"

	"github.com/MJDSystems/kilium/kilium"
	"github.com/MJDSystems/kilium/kiliumtmp"
)

func main() {
	idGen := make(chan uint64)

	go func() {
		start := uint64(time.Now().Unix() << 20)
		log.Println(start>>20 - uint64(time.Now().Unix()))
		for {
			idGen <- start
			start++
		}
	}()

	con, err := kilium.GetDatabaseConnection("localhost:10017")
	if err != nil {
		log.Panic(err)
	}
	master := kilium.NewRssMaster(con, idGen)
	_ = master

	for _, Url := range kiliumtmp.FeedList {
		master.AddRequestCh <- kilium.AddFeedRequest{Url, make(chan error, 1)}
	}
	log.Println(<-idGen)
	for {
		<-time.Tick(time.Hour * 24 * 365)
	}
}
