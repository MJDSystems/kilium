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

	"math/rand"
	"reflect"

	"strconv"
	"time"

	"testing"
	"testing/quick"
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
		item.GenericKey = makeHash(string(item.GenericKey) + key_uniquer)
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

	type FeedItemCh struct {
		item FeedItem
		ch   chan FeedItemCh
	}

	itemChOut := make(chan FeedItemCh)
	itemCh := make(chan FeedItem)

	go func(itemCh chan FeedItem, itemChOut chan FeedItemCh) {
		defer close(itemCh)
		for item, ok := <-itemChOut; ok; item, ok = <-itemChOut {
			itemCh <- item.item
			itemChOut = item.ch
		}
	}(itemCh, itemChOut)

	itemChIn := make(chan FeedItemCh)

	for _, itemKey := range model.ItemKeys {
		go func(itemKey ItemKey, itemChOut, itemChIn chan FeedItemCh) {
			defer close(itemChOut)

			modelItem := FeedItem{}
			if err := con.LoadModel(itemKey.GetRiakKey(), &modelItem, riak.R1); err != nil {
				t.Errorf("Failed to load item! Error: %s item %s", err, itemKey.GetRiakKey())
			}
			itemChOut <- FeedItemCh{modelItem, itemChIn}
		}(itemKey, itemChOut, itemChIn)
		itemChOut = itemChIn
		itemChIn = make(chan FeedItemCh)
	}

	//Compare saved feed items.  This means a trip through riak!  The order should match though ...
	for i, _ := range model.ItemKeys {
		var modelItem FeedItem
		select {
		case modelItem = <-itemCh:
		case <-time.After(time.Minute * 5):
			t.Fatalf("Failed to get an item before timeout, item %v", i)
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

func checkAllItemsDeleted(t *testing.T, feed *Feed, con *riak.Client) bool {
	ch := make(chan bool)
	for _, itemKey := range feed.ItemKeys {
		go func(itemKey ItemKey, ch chan<- bool) {
			modelItem := FeedItem{}
			if err := con.LoadModel(itemKey.GetRiakKey(), &modelItem, riak.R1); err == riak.NotFound {
				ch <- false
			} else {
				ch <- true
			}
		}(itemKey, ch)
	}

	problems := 0
	for _, _ = range feed.ItemKeys {
		found := <-ch
		if found {
			//t.Errorf("Found deleted item %s", itemKey.GetRiakKey())
			problems++
		}
	}
	return problems == 0
}

func CreateFeed(t *testing.T, con *riak.Client, Url *url.URL) *Feed {
	feedModel := &Feed{Url: *Url}
	if err := con.LoadModel(feedModel.UrlKey(), feedModel); err == nil && err != riak.NotFound {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else {
		modelElement := feedModel.Model
		*feedModel = Feed{Url: *Url}
		feedModel.Model = modelElement
		if err = feedModel.Save(); err != nil {
			t.Fatalf("Failed to store feed model (%s)!", err)
		}
	}
	return feedModel
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

var urlKeyRand = rand.New(rand.NewSource(0))

func getUniqueExampleComUrl(t *testing.T) *url.URL {
	url, err := url.Parse("http://example.com/" + key_uniquer + strconv.Itoa(urlKeyRand.Int()) + "/rss")
	if err != nil {
		t.Fatalf("Failed to generate url (%s)", err)
	}
	return url
}

func TestSingleFeedInsert(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	feed := getFeedDataFor(t, "simple", 0)
	fixFeedForMerging(feed)

	url := getUniqueExampleComUrl(t)

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

	url := getUniqueExampleComUrl(t)

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

	url := getUniqueExampleComUrl(t)

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

	url := getUniqueExampleComUrl(t)

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

func GenerateParsedFeed(rand *rand.Rand) (out ParsedFeedData) {
	out = ParsedFeedData{}
	if titleOut, ok := quick.Value(reflect.TypeOf(out.Title), rand); ok {
		out.Title = titleOut.Interface().(string)
	} else {
		panic("Couldn't make a title!")
	}

	out.Items = make([]ParsedFeedItem, MaximumFeedItems+20)
	for i, _ := range out.Items {
		out.Items[i].GenericKey = makeHash(key_uniquer + strconv.Itoa(rand.Int()))
		out.Items[i].Title = strconv.Itoa(i)
	}

	return
}

func TestFeedDealingWithOverLargeFeed(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	rand := rand.New(rand.NewSource(0))

	url := getUniqueExampleComUrl(t)
	feedModel := CreateFeed(t, con, url)

	t.Log("Test creating overlarge feed.")

	x := GenerateParsedFeed(rand)
	if err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}

	x.Items = x.Items[:MaximumFeedItems]

	origLoadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), origLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, origLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data (minus overage!)")
	}

	t.Log("Test completely replacing said feed with a new Oversized feed.")

	x = GenerateParsedFeed(rand) // This should generate all new items.
	if err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to replace simple single feed (%s)!", err)
	}

	x.Items = x.Items[:MaximumFeedItems]

	newLoadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data (minus overage!)")
	} else if checkAllItemsDeleted(t, origLoadFeed, con) == false {
		t.Fatalf("There are left over deleted items!")
	}

	t.Log("Test adding a few new entries")

	newFeedData := getFeedDataFor(t, "simple", 0)
	fixFeedForMerging(newFeedData)
	fullItems := x.Items
	x.Items = newFeedData.Items

	if err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}
	x.Items = append(x.Items, fullItems[:MaximumFeedItems-len(newFeedData.Items)]...)
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data (minus overage!)")
	}
}
