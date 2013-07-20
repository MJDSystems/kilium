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
	"fmt"
	"sort"

	"time"

	"testing"
)

func compareParsedFeedData(original ParsedFeedData, parsed ParsedFeedData, fetchTime time.Time) bool {
	return customDeepEqual(original, parsed, []string{"NextCheckTime"}) &&
		// Verify this seperately since the original data can't contain the right value.
		parsed.NextCheckTime.Equal(fetchTime.Add(time.Hour))
}

// Note, original is modified to preform the compare operations!
func verifyParsedAtomFeed(t *testing.T, original, parsed ParsedFeedData, fetchTime time.Time) {
	origItems := original.Items
	original.Items = make([]ParsedFeedItem, len(origItems))
	copy(original.Items, origItems)
	sort.Sort(original.Items)

	for i, _ := range original.Items {
		item := &original.Items[i]
		if len(item.GenericKey) == 0 {
			item.GenericKey = makeHash(fmt.Sprintf("tag:%s,%s:%s", item.Url.Host, item.PubDate.Format("2006-01-02"), item.Url.Path))
		} else {
			item.GenericKey = makeHash(string(item.GenericKey))
		}
	}

	if !compareParsedFeedData(original, parsed, fetchTime) {
		t.Errorf("Failed to properly parse atom feed,\nOriginal:\n(%+v)\nParsed:\n(%+v)", original, parsed)
	}
}

// Note, original is modified to preform the compare operations!
func verifyParsedRssFeed(t *testing.T, original, parsed ParsedFeedData, fetchTime time.Time) {
	origItems := original.Items
	original.Items = make([]ParsedFeedItem, len(origItems))
	copy(original.Items, origItems)
	sort.Sort(original.Items)

	for i, _ := range original.Items {
		item := &original.Items[i]
		if len(item.GenericKey) == 0 {
			if item.Url.String() != "" {
				item.GenericKey = []byte(item.Url.String())
			} else if item.Title != "" {
				item.GenericKey = []byte(item.Title)
			} else if item.Content != "" {
				item.GenericKey = []byte(item.Content)
			} else if !item.PubDate.IsZero() {
				item.GenericKey = []byte(item.PubDate.String())
			}
		}
		item.GenericKey = makeHash(string(item.GenericKey))
	}

	if !compareParsedFeedData(original, parsed, fetchTime) {
		t.Errorf("Failed to properly parse rss feed,\nOriginal:\n(%+v)\nParsed:\n(%+v)", original, parsed)
	}
}

func TestSimpleFeedParserTest(t *testing.T) {
	fetchTime := time.Now()

	atom, _ := produceFeedStructureFromData(getFeedDataFor(t, "simple", 0)).ToAtom()
	rss, _ := produceFeedStructureFromData(getFeedDataFor(t, "simple", 0)).ToRss()

	atomOut, e := parseRssFeed([]byte(atom), fetchTime)
	if e != nil {
		t.Fatalf("Failed to parse atom feed (%s)", e)
	}
	rssOut, e := parseRssFeed([]byte(rss), fetchTime)
	if e != nil {
		t.Fatalf("Failed to parse atom feed (%s)", e)
	}
	feed := getFeedDataFor(t, "simple", 0)

	verifyParsedAtomFeed(t, *feed, *atomOut, fetchTime)
	verifyParsedRssFeed(t, *feed, *rssOut, fetchTime)
}
