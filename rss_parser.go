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
	"crypto/sha512"

	"io"

	"net/url"

	"sort"

	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"

	rss "github.com/jteeuwen/go-pkg-rss"
)

type ParsedFeedItemList []ParsedFeedItem

type ParsedFeedData struct {
	Title string

	Items ParsedFeedItemList

	NextCheckTime time.Time
}

type ParsedFeedItem struct {
	GenericKey []byte //This is just the hash of the GUID or Link or Title or w/e makes it unique.  It is not the system's id, as it doesn't include the generated id
	Title      string
	Author     string
	Content    string

	Url url.URL

	PubDate time.Time
}

func makeHash(str string) []byte {
	hasher := sha512.New()

	n, err := io.WriteString(hasher, str)
	if err != nil {
		panic(err)
	} else if n != len(str) {
		panic(n)
	}

	return hasher.Sum(nil)
}

func (l ParsedFeedItemList) Len() int {
	return len(l)
}

func (l ParsedFeedItemList) Less(i, j int) bool {
	return l[i].PubDate.After(l[j].PubDate) //The latest item should be at position 0
}

func (l ParsedFeedItemList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func parseRssFeed(feedContents []byte) (*ParsedFeedData, error) {
	feed := rss.New(0, true, nil, nil)

	err := feed.FetchBytes("", feedContents, charset.NewReader)
	if err != nil {
		return nil, err
	}

	channel := feed.Channels[0]

	output := ParsedFeedData{
		Title: channel.Title,
	}

	for _, item := range channel.Items {
		nextItem := ParsedFeedItem{
			Title:  item.Title,
			Author: item.Author.Name,
		}
		if item.Content == nil {
			nextItem.Content = item.Description
		} else {
			nextItem.Content = item.Content.Text
		}

		if len(item.Links) >= 1 {
			if Url, err := url.Parse(item.Links[0].Href); err != nil {
				// Note need to log these failures for the future
				nextItem.Url = url.URL{}
			} else {
				nextItem.Url = *Url
			}
		}

		if item.PubDate != "" {
			if Date, err := parseDate(item.PubDate); err != nil {
				// Also log this for the future ...
				return nil, err
			} else {
				nextItem.PubDate = Date
			}
		}

		// The item id is a SHA512 hash of one of the following, whatever comes first:
		// GUID, Link (as used above), Title, Content, PubDate (from a processed state above).
		// Failing that any of those are useful, just fail the item.
		if item.Guid != "" {
			nextItem.GenericKey = makeHash(item.Guid)
		} else if len(item.Links) >= 1 {
			nextItem.GenericKey = makeHash(item.Links[0].Href)
		} else if item.Title != "" {
			nextItem.GenericKey = makeHash(item.Title)
		} else if item.Content != nil && item.Content.Text != "" {
			nextItem.GenericKey = makeHash(item.Content.Text)
		} else if !nextItem.PubDate.IsZero() {
			nextItem.GenericKey = makeHash(nextItem.PubDate.String())
		} else {
			continue //Seriously, this should be reported.  The item is probably junk ...
		}

		output.Items = append(output.Items, nextItem)
	}

	sort.Sort(output.Items)

	return &output, nil
}

type FeedParserOut struct {
	Data ParsedFeedData
	Url  url.URL
}

func FeedParser(in <-chan RawFeed, out chan<- FeedParserOut, errChan chan<- FeedError) {
	for {
		if next, ok := <-in; ok {

			data, err := parseRssFeed(next.Data)
			if err == nil {
				out <- FeedParserOut{Data: *data, Url: next.Url}
			} else {
				errChan <- FeedError{Err: err, Url: next.Url}
			}
		} else {
			break
		}
	}
}
