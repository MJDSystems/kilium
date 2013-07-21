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
	if !data.NextCheckTime.Equal(model.NextCheck) {
		t.Errorf("Next time to check feed doesn't match %#v vs %#v!", data.NextCheckTime, model.NextCheck)
	}
	if !(strconv.FormatInt(data.NextCheckTime.Unix(), 10) == model.Indexes()[LastCheckIndexName]) {
		t.Errorf("Next time(in 2i) to check feed doesn't match %v vs %v!", data.NextCheckTime.Unix(), model.Indexes()[LastCheckIndexName])
	}
	if !data.FetchedAt.Equal(model.LastCheck) {
		t.Errorf("Fetch time from feed doesn't match %#v vs %#v!", data.NextCheckTime, model.NextCheck)
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

	for _, itemKey := range model.ItemKeys {
		itemChIn := make(chan FeedItemCh)

		go func(itemKey ItemKey, itemChOut, itemChIn chan FeedItemCh) {
			defer close(itemChOut)

			modelItem := FeedItem{}
			if err := con.LoadModel(itemKey.GetRiakKey(), &modelItem, riak.R1); err != nil {
				t.Errorf("Failed to load item! Error: %s item %s", err, itemKey.GetRiakKey())
			}
			itemChOut <- FeedItemCh{modelItem, itemChIn}
		}(itemKey, itemChOut, itemChIn)

		itemChOut = itemChIn
	}
	// This ensures the feeder go routine will eventually quit.  It either closes its initial input,
	// or the final channel it will get (since itemChOut is equal to the last channel the go routine
	// will read from, since it stores the last itemChIn from above).
	close(itemChOut)

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

func checkAllItemsDeleted(t *testing.T, itemKeyList ItemKeyList, con *riak.Client) bool {
	ch := make(chan bool)
	for _, itemKey := range itemKeyList {
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
	for _, _ = range itemKeyList {
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
	if err := con.LoadModel(feedModel.UrlKey(), feedModel); err != nil && err != riak.NotFound {
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

		if _, err := updateFeed(con, *url, *feed, testIdGenerator); err != nil {
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

	_, err := updateFeed(con, *url, *feed, testIdGenerator)
	if err != FeedNotFound {
		t.Fatalf("Failed to insert simple single feed (%s)!", err)
	}

	feedModel := CreateFeed(t, con, url)

	updatedFeed, err := updateFeed(con, *url, *feed, testIdGenerator)
	if err != nil {
		t.Errorf("Failed to update simple single feed (%s)!", err)
	}

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err = con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else {
		if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
			t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
		}
		if !reflect.DeepEqual(loadFeed, updatedFeed) {
			t.Errorf("Returned model from saving differs from loaded model! Original:\n%+v\nLoaded:\n%+v", updatedFeed, loadFeed)
		}
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

func mustCreateEmptyItemAt(t *testing.T, con *riak.Client, itemKey ItemKey) {
	itemModel := FeedItem{}
	if err := con.LoadModel(itemKey.GetRiakKey(), &itemModel); err != riak.NotFound {
		t.Fatalf("Failed to preload item to create an empty item at %s (%s)", itemKey.GetRiakKey(), err)
	} else if err = itemModel.Save(); err != nil {
		t.Fatalf("Failed to save an empty item at %s (%s)", itemKey.GetRiakKey(), err)
	}
}

func TestWithExistingToDeleteItems(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url := getUniqueExampleComUrl(t)

	feedModel := CreateFeed(t, con, url)

	feedModel.Title = "ASDF"
	feedModel.DeletedItemKeys = ItemKeyList{
		NewItemKey(1, makeHash("DKey 1"+key_uniquer)),
		NewItemKey(2, makeHash("DKey 2 - DNE"+key_uniquer)),
		NewItemKey(3, makeHash("DKey 3"+key_uniquer)),
		NewItemKey(4, makeHash("DKey 4 - DNE"+key_uniquer)),
	}
	mustCreateEmptyItemAt(t, con, feedModel.DeletedItemKeys[0])
	mustCreateEmptyItemAt(t, con, feedModel.DeletedItemKeys[2])
	deletedItems := feedModel.DeletedItemKeys

	if err := feedModel.Save(); err != nil {
		t.Fatalf("Failed to save preloaded content!")
	}

	feed := &ParsedFeedData{}
	if _, err := updateFeed(con, *url, *feed, testIdGenerator); err != nil {
		t.Errorf("Failed to update simple single feed (%s)!", err)
	}

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else {
		if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
			t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
		}
		if checkAllItemsDeleted(t, deletedItems, con) == false {
			t.Errorf("There are left over deleted items!")
		}
	}
}

func TestWithExistingToInsertItems(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url := getUniqueExampleComUrl(t)

	feedModel := CreateFeed(t, con, url)

	feedModel.Title = "ASDF"
	feedModel.InsertedItemKeys = ItemKeyList{
		NewItemKey(1, makeHash("IKey 1"+key_uniquer)),
		NewItemKey(2, makeHash("IKey 2 - DNE"+key_uniquer)),
		NewItemKey(3, makeHash("IKey 3"+key_uniquer)),
		NewItemKey(4, makeHash("IKey 4 - DNE"+key_uniquer)),
	}
	mustCreateEmptyItemAt(t, con, feedModel.InsertedItemKeys[0])
	mustCreateEmptyItemAt(t, con, feedModel.InsertedItemKeys[2])

	if err := feedModel.Save(); err != nil {
		t.Fatalf("Failed to save preloaded content!")
	}

	feed := &ParsedFeedData{}
	if _, err := updateFeed(con, *url, *feed, testIdGenerator); err != nil {
		t.Errorf("Failed to update simple single feed (%s)!", err)
	}

	// Need to mention my empty items.  Not used before the insert call, as that could invalidate
	// this whole test!
	feed.Items = []ParsedFeedItem{
		ParsedFeedItem{},
		ParsedFeedItem{},
	}

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}

func TestTwoItemsOneKey(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url := getUniqueExampleComUrl(t)

	feedModel := CreateFeed(t, con, url)

	feed := MustUpdateFeedTo(t, con, url, "duplicates", 1)
	feed.Items = feed.Items[:1]

	// Finally, load the feed again and verify properties!
	loadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), loadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, feed, loadFeed, con) != true {
		t.Errorf("Saved feed does not match what was inserted! Original:\n%+v\nLoaded:\n%+v", feed, loadFeed)
	}
}

func TestFeedDealingWithOverLargeFeed(t *testing.T) {
	// This test takes forever, so skip it if I don't care for my long tests.
	if testing.Short() {
		t.SkipNow()
	}

	con := getTestConnection(t)
	defer killTestDb(con, t)

	rand := rand.New(rand.NewSource(0))

	url := getUniqueExampleComUrl(t)
	feedModel := CreateFeed(t, con, url)

	t.Log("Test creating overlarge feed.")

	x := GenerateParsedFeed(rand)
	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
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
	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to replace simple single feed (%s)!", err)
	}

	x.Items = x.Items[:MaximumFeedItems]

	newLoadFeed := &Feed{}
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data (minus overage!)")
	} else if checkAllItemsDeleted(t, origLoadFeed.ItemKeys, con) == false {
		t.Fatalf("There are left over deleted items!")
	}

	t.Log("Test adding a few new entries")

	newFeedData := getFeedDataFor(t, "simple", 0)
	fixFeedForMerging(newFeedData)
	fullItems := x.Items
	x.Items = newFeedData.Items

	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}
	x.Items = append(x.Items, fullItems[:MaximumFeedItems-len(newFeedData.Items)]...)
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data (minus overage!)")
	}

	t.Log("Update existing items")

	newFeedData = getFeedDataFor(t, "simple", 1)
	fixFeedForMerging(newFeedData)
	fullItems = x.Items[3:] // Kill the first four items, as they are what are going to end up updated.
	x.Items = newFeedData.Items

	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}
	x.Items = append(x.Items, fullItems...)
	x.Items = x.Items[:MaximumFeedItems]
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data + updates")
	}

	t.Log("Update + insert existing items")

	newFeedData = getFeedDataFor(t, "simple", 2)
	fixFeedForMerging(newFeedData)
	fullItems = x.Items[3:] // Kill the first five items, as they are what are going to end up updated.
	x.Items = newFeedData.Items

	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}
	x.Items = append(x.Items, fullItems...)
	x.Items = x.Items[:MaximumFeedItems]
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data + updates")
	}

	t.Log("Insert undersized to make full (from max-1 + 2 = max)")

	//First use the loaded model above to kick off an element.
	newLoadFeed.ItemKeys = newLoadFeed.ItemKeys[:len(newLoadFeed.ItemKeys)-1]
	newLoadFeed.Save()
	fullItems = x.Items[:len(newLoadFeed.ItemKeys)]
	// Now, using the above feed data, stick two items in.  Give them a pubdate far into the future,
	// with a new unused key.
	x.Items = make([]ParsedFeedItem, 2)
	x.Items[0].GenericKey = makeHash(key_uniquer + "_New_1")
	x.Items[0].PubDate = time.Now().Add(time.Hour * 24 * 365 * 100) // Make it far into the future!
	x.Items[0].GenericKey = makeHash(key_uniquer + "_New_2")
	x.Items[0].PubDate = time.Now().Add(time.Hour*24*365*100 - 1) // Make it far into the future!

	if _, err := updateFeed(con, *url, x, testIdGenerator); err != nil {
		t.Fatalf("Failed to update simple single feed (%s)!", err)
	}
	x.Items = append(x.Items, fullItems...)
	x.Items = x.Items[:MaximumFeedItems]
	if err := con.LoadModel(feedModel.UrlKey(), newLoadFeed); err != nil {
		t.Fatalf("Failed to initialize feed model (%s)!", err)
	} else if compareParsedToFinalFeed(t, &x, newLoadFeed, con) == false {
		t.Fatalf("Inserted data did not match original data + updates")
	} else if len(newLoadFeed.ItemKeys) != MaximumFeedItems {
		t.Fatalf("Somehow, didn't get the full feed back!")
	}
}
