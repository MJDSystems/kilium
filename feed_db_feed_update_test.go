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

	"testing"
)

func makeIdGenerator() <-chan uint64 {
	ch := make(chan uint64)

	go func(ch chan<- uint64) {
		var id uint64
		for {
			ch <- id
			id++
		}
	}(ch)

	return ch
}

var testIdGenerator <-chan uint64 = makeIdGenerator()

func fixFeedForMerging(toFix *ParsedFeedData) *ParsedFeedData {
	// Instead of sorting, verify we are sorted.  Otherwise items without publication dates can end
	// up in the wrong place.
	if !toFix.Items.VerifySort() {
		panic("Items are not sorted!!!!")
	}

	for i, _ := range toFix.Items {
		item := &toFix.Items[i]
		item.GenericKey = makeHash(string(item.GenericKey))
	}
	return toFix
}

func compareParsedToFinalFeed(t *testing.T, data *ParsedFeedData, model *Feed, con *riak.Client) bool {
	// Compare basics:
	if data.Title != model.Title {
		t.Errorf("Feed title didn't match '%s' vs '%s'!", data.Title, model.Title)
		return false
	}

	if len(data.Items) != len(model.ItemKeys) {
		if len(data.Items) > MaximumFeedItems {
			t.Errorf("Item count differs due to items count greater then Maximum number of feed items (%v of %v)", len(data.Items), MaximumFeedItems)
		} else {
			t.Errorf("Item count is different %v vs %v!", len(data.Items), len(model.ItemKeys))
		}
		return false
	}
	if len(model.InsertedItemKeys) != 0 || len(model.DeletedItemKeys) != 0 {
		t.Error("There are left over inserted or deleted item keys!")
		return false
	}

	//Compare saved feed items.  This means a trip through riak!  The order should match though ...
	for i, itemKey := range model.ItemKeys {
		modelItem := FeedItem{}
		if err := con.LoadModel(string(itemKey), &modelItem); err != nil {
			t.Errorf("Failed to load item! Error: %s", err)
			return false
		}

		dataItem := data.Items[i]

		if dataItem.Title != modelItem.Title ||
			dataItem.Author != modelItem.Author ||
			dataItem.Content != modelItem.Content ||
			dataItem.Url != modelItem.Url ||
			!dataItem.PubDate.Equal(modelItem.PubDate) {
			t.Errorf("Item data didn't match! Original:\n%#v\nLoaded:\n%#v", dataItem, modelItem)
			return false
		}
	}

	return true
}

func CreateFeed(t *testing.T, con *riak.Client, Url *url.URL) Feed {
	feedModel := &Feed{Url: *Url}
	if err := con.NewModel(feedModel.UrlKey(), feedModel); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if err = feedModel.Save(); err != nil {
		t.Fatalf("Failed to store feed model (%s)!", err)
	}
	return *feedModel
}

func MustUpdateFeedTo(t *testing.T, con *riak.Client, url *url.URL, feedName string, updateCount int) (feed *ParsedFeedData) {
	for i := 0; i < updateCount; i++ {
		feed = getFeedDataFor(t, feedName, i)
		fixFeedForMerging(feed)

		if err := updateFeed(con, *url, *feed, testIdGenerator); err != nil {
			t.Fatalf("Failed to update simple single feed (%s)!", err)
		}
	}

	return
}

func TestSingleFeedInsert(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	feed := getFeedDataFor(t, "simple", 0)
	fixFeedForMerging(feed)

	url, _ := url.Parse("http://example.com/rss")

	err := updateFeed(con, *url, *feed, testIdGenerator)
	if err != FeedNotFound {
		t.Fatalf("Failed to insert simple single feed (%s)!", err)
	}

	feedModel := CreateFeed(t, con, url)

	err = updateFeed(con, *url, *feed, testIdGenerator)
	if err != nil {
		t.Errorf("Failed to update simple single feed (%s)!", err)
	}

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err = con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}

func TestSingleFeedUpdate(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url, _ := url.Parse("http://example.com/rss")

	feedModel := CreateFeed(t, con, url)

	feed := MustUpdateFeedTo(t, con, url, "simple", 2)

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}

func TestFeedUpdateAndInsert(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url, _ := url.Parse("http://example.com/rss")

	feedModel := CreateFeed(t, con, url)

	feed := MustUpdateFeedTo(t, con, url, "simple", 3)

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}

func TestFeedUpdateWithChangingPubDates(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url, _ := url.Parse("http://example.com/rss")

	feedModel := CreateFeed(t, con, url)

	feed := MustUpdateFeedTo(t, con, url, "simple", 4)

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}
