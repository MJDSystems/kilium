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
package main

import (
	"net/url"

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

	return
}
