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
	"errors"
	"net/url"
	"time"

	"strconv"
	"testing"
)

func TestRssMasterHandleAddRequest(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Url := getUniqueExampleComUrl(t)

	loadFeed := &Feed{Url: *Url}

	if err := RssMasterHandleAddRequest(con, *Url); err != nil {
		t.Fatalf("Failed to create fresh new feed! (%s)", err)
	} else if err = con.LoadModel(loadFeed.UrlKey(), loadFeed); err != nil {
		t.Errorf("Failed to load just created model! (%s)", err)
	} else if !loadFeed.NextCheck.IsZero() {
		t.Errorf("Next check time on empty feed isn't zero! (Is: %s)", loadFeed.NextCheck)
	} else if loadFeed.Indexes()[NextCheckIndexName] != strconv.FormatInt(loadFeed.NextCheck.Unix(), 10) {
		t.Errorf("Next check time index on empty feed isn't zero! (Is: %v)", loadFeed.Indexes()[NextCheckIndexName])
	}

	loadFeed.NextCheck = loadFeed.NextCheck.Add(time.Hour * 24 * 365 * 200) //Bring us to something more recent.
	if err := loadFeed.Save(); err != nil {
		t.Fatalf("Failed to change the feed!")
	}

	loadFeed = &Feed{Url: *Url} // Reset the loaded feed

	if err := RssMasterHandleAddRequest(con, *Url); err != nil {
		t.Errorf("Failed to re-request new feed! (%s)", err)
	} else if err = con.LoadModel(loadFeed.UrlKey(), loadFeed); err != nil {
		t.Errorf("Failed to load new feed! (%s)", err)
	} else if loadFeed.NextCheck.IsZero() {
		t.Errorf("Next check time on empty feed is zero! (Is: %s)", loadFeed.NextCheck)
	}
}

func TestRssMasterPollSingleFeed(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Url := getUniqueExampleComUrl(t)

	if err := RssMasterHandleAddRequest(con, *Url); err != nil {
		t.Fatalf("Failed to create fresh new feed! (%s)", err)
	}

	// Make channels for requests, and start a fake handler that alternates between sending success
	// and failures.  Failures are left largely unhandled, but at least we should see the test finish!
	inputCh := make(chan url.URL)
	defer close(inputCh)
	outputCh := make(chan FeedError)

	feedsParsed := 0

	go func(inputCh <-chan url.URL, outputCh chan<- FeedError) {
		defer close(outputCh)
		for {
			if next, ok := <-inputCh; !ok {
				break
			} else {
				feedsParsed++
				outputCh <- FeedError{Url: next, Err: nil} // SUCCESS!!!
			}

			if next, ok := <-inputCh; !ok {
				break
			} else {
				feedsParsed++
				outputCh <- FeedError{Url: next, Err: errors.New("Random test failure!")} // FAILURE!!!
			}
		}
	}(inputCh, outputCh)

	// And try the fetch!
	RssMasterPollFeeds(con, inputCh, outputCh)
	t.Log(feedsParsed)
	if feedsParsed != 1 {
		t.Errorf("Failed to parse the expected number of feeds.  Wanted 1, got %v", feedsParsed)
	}
}
