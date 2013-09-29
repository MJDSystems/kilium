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
	"encoding/json"
	"io/ioutil"
	"strconv"

	"testing"

	"github.com/gorilla/feeds"
)

const (
	numberOfFeedInputFile = 1
	feedDir               = "test_feed_data"
	baseFile              = "feed_"
	totalBaseFile         = feedDir + "/" + baseFile
)

func produceFeedStructureFromData(d *ParsedFeedData) (ret *feeds.Feed) {
	ret = &feeds.Feed{}
	ret.Title = d.Title
	ret.Link = &feeds.Link{}
	ret.Author = &feeds.Author{d.Items[0].Author, "asdf"}

	for _, item := range d.Items {
		ret.Items = append(ret.Items, &feeds.Item{
			Id:          string(item.GenericKey),
			Title:       item.Title,
			Author:      &feeds.Author{item.Author, "asdf"},
			Description: item.Content,
			Link:        &feeds.Link{Href: item.Url.String()},
			Updated:     item.PubDate,
		})
	}
	return
}

func getFeedDataFor(t *testing.T, feed int) (parsedFeed *ParsedFeedData) {
	if feed >= numberOfFeedInputFile {
		t.Fatalf("Requested non-existant feed file %v of %v", feed, numberOfFeedInputFile)
	}

	d, err := ioutil.ReadFile(totalBaseFile + strconv.Itoa(feed) + ".json")

	parsedFeed = &ParsedFeedData{}
	err = json.Unmarshal(d, parsedFeed)

	if err != nil {
		t.Fatalf("Failed to unmarshal json (%s)!", err)
	}

	return
}
