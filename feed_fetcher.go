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
	"io/ioutil"

	"net/http"
	"net/url"

	"time"
)

type RawFeed struct {
	Data      []byte
	Url       url.URL
	FetchedAt time.Time
}

type FeedError struct {
	Err error
	Url url.URL
}

func FeedFetcher(in <-chan url.URL, out chan<- RawFeed, errChan chan<- FeedError) {
	for {
		if next, ok := <-in; ok {
			resp, err := http.Get(next.String())
			if err != nil {
				errChan <- FeedError{err, next}
				continue
			}

			content, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			if err != nil {
				errChan <- FeedError{err, next}
			} else {
				out <- RawFeed{content, next, time.Now()}
			}
		} else {
			break
		}
	}
}
