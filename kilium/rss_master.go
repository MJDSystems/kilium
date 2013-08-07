/*
 * Copyright (C) 2013 Matthew Dawson <matthew@mjdsystems.ca>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package kilium

import (
	"log"

	"net/url"
	"strconv"
	"time"

	riak "github.com/tpjg/goriakpbc"
)

type AddFeedRequest struct {
	Url        url.URL
	ResponseCh chan<- error
}

type RssMaster struct {
	AddRequestCh chan<- AddFeedRequest

	pipeline RssParserPipeline
}

func RssMasterHandleAddRequest(con *riak.Client, Url url.URL) error {
	feedModel := &Feed{Url: Url}
	if err := con.LoadModel(feedModel.UrlKey(), feedModel); err != nil && err != riak.NotFound {
		return err
	} else if err == nil {
		return nil
	} else { // Implicitly err == riak.NotFound
		feedModel.Indexes()[NextCheckIndexName] = strconv.FormatInt(time.Time{}.Unix(), 10)
		if err = feedModel.Save(); err != nil {
			return err
		}
	}
	return nil
}

func RssMasterPollFeeds(con *riak.Client, InputCh chan<- url.URL, OutputCh <-chan FeedError) {
	bucket, err := con.NewBucket("feeds")
	if err != nil {
		log.Println("Failed to get feed bucket:", err)
	}
	// -62135596800 is Go's zero time according to Unix's time format.  This is what empty feeds have for their check time.
	// Nothing should appear before that.
	keys_to_poll, err := bucket.IndexQueryRange(NextCheckIndexName, "-62135596800", strconv.FormatInt(time.Now().Unix(), 10))
	var errors []error

	valid_keys := 0
	for _, key := range keys_to_poll {
		var loadFeed Feed
		if err := con.LoadModel(key, &loadFeed); err != nil {
			errors = append(errors, err)
		} else {
			log.Println(loadFeed.Url)
			valid_keys++
			go func(Url url.URL, inputCh chan<- url.URL) {
				inputCh <- Url
			}(loadFeed.Url, InputCh)
		}
	}
	for i := 0; i < valid_keys; i++ {
		if err := <-OutputCh; err.Err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) != 0 {
		log.Println(MultiError(errors))
	}
}

func NewRssMaster(con *riak.Client, idGenerator <-chan uint64) (master RssMaster) {
	AddRequestCh := make(chan AddFeedRequest)
	master = RssMaster{
		AddRequestCh: AddRequestCh,

		pipeline: NewRssParserPipeline(con, idGenerator),
	}

	go func(con *riak.Client, AddRequestCh <-chan AddFeedRequest) {
		for {
			next, ok := <-AddRequestCh
			if !ok {
				break
			}
			next.ResponseCh <- RssMasterHandleAddRequest(con, next.Url)
		}
	}(con, AddRequestCh)
	go func(con *riak.Client, InputCh chan<- url.URL, OutputCh <-chan FeedError) {
		tick := time.Tick(5 * time.Minute)
		for {
			RssMasterPollFeeds(con, InputCh, OutputCh)
			<-tick //Pause and wait for 5 minutes to pass.  This is just to batch requests when possible.
		}
	}(con, master.pipeline.InputCh, master.pipeline.OutputCh)

	return
}
