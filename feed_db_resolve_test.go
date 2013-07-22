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
	"bytes"
	"encoding/binary"

	"net/url"

	"reflect"

	"testing"

	"strconv"
	"time"

	riak "github.com/tpjg/goriakpbc"
)

var testFeedUrl, _ = url.Parse("http://example.com/feed.rss")

func compareItems(i1, i2 FeedItem) bool {
	return customDeepEqual(i1, i2, []string{"Model"})
}

func TestFeedAtributesResolving(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	EntryA := Feed{
		Url: *testFeedUrl,

		Title:     "First Title",
		LastCheck: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),

		NextCheck: time.Date(2013, 7, 1, 1, 0, 0, 0, time.UTC),
		//No items, because I don't care.
	}
	EntryB := Feed{
		Url: *testFeedUrl,

		Title:     "Second Title",
		LastCheck: time.Date(2013, 7, 1, 12, 0, 0, 0, time.UTC), //This is of course a second check date.

		NextCheck: time.Date(2013, 7, 1, 13, 0, 0, 0, time.UTC),
		//No items, because I don't care.
	}

	if err := con.NewModel("ConflictFeed", &EntryA); err != nil {
		t.Fatalf("Failed to create EntryA's model (%s)", err)
	}
	EntryA.Indexes()[NextCheckIndexName] = strconv.FormatInt(EntryA.NextCheck.Unix(), 10)
	if err := EntryA.Save(); err != nil {
		t.Fatalf("Failed to save EntryA (%s)", err)
	}

	if err := con.NewModel("ConflictFeed", &EntryB); err != nil {
		t.Fatalf("Failed to create EntryB's model (%s)", err)
	}
	EntryB.Indexes()[NextCheckIndexName] = strconv.FormatInt(EntryB.NextCheck.Unix(), 10)
	if err := EntryB.Save(); err != nil {
		t.Fatalf("Failed to save EntryB (%s)", err)
	}

	// Cause conflict
	load := Feed{}
	if err := con.LoadModel("ConflictFeed", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	} else {
		if customDeepEqual(EntryB, load, []string{"Model", "ItemKeys", "DeletedItemKeys", "InsertedItemKeys"}) == false {
			t.Errorf("Resolved model does not match latest update (old, new) (\n%+v, \n%+v)", EntryB, load)
		}
		if reflect.DeepEqual(EntryB.Indexes(), load.Indexes()) == false {
			t.Errorf("Resolved model's indexes don't match latest update (old, new) (\n%+v, \n%+v)", EntryB.Indexes(), load.Indexes())
		}
	}
}

func genItemKey(id int64, uuid string) []byte {
	buf := &bytes.Buffer{}

	binary.Write(buf, binary.BigEndian, id)
	buf.WriteString("-" + uuid)

	return buf.Bytes()
}

func TestFeedItemsResolvingSimple(t *testing.T) { // Only ensures the lists turn out what I want.
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Entry := Feed{
		Url: *testFeedUrl,

		Title:     "First Title",
		LastCheck: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),

		NextCheck: time.Date(2013, 7, 1, 1, 0, 0, 0, time.UTC),

		ItemKeys:        ItemKeyList{genItemKey(10, "A"), genItemKey(8, "B")},
		DeletedItemKeys: ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")},

		InsertedItemKeys: ItemKeyList{genItemKey(10, "IA"), genItemKey(8, "IB")},
	}

	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Using same base data, add more keys.  Pretend A fell off a cliff, and add an E.  DeletedItems clears.
	Entry.ItemKeys = ItemKeyList{genItemKey(100, "AA"), genItemKey(8, "B"), genItemKey(2, "E")}
	Entry.InsertedItemKeys = ItemKeyList{genItemKey(8, "IB"), genItemKey(2, "IE")}
	Entry.DeletedItemKeys = ItemKeyList{}

	// Clear the model to make a sibling.
	Entry.Model = riak.Model{}
	//And save
	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Cause conflict
	load := Feed{}
	if err := con.LoadModel("ConflictFeed", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	}

	// Verify lists.
	test := ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")}
	if reflect.DeepEqual(load.DeletedItemKeys, test) != true {
		t.Errorf("Deleted Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.DeletedItemKeys, ItemKeyList{genItemKey(40, "C"), genItemKey(30, "D")})
	}
	test = ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "A"), genItemKey(8, "B"), genItemKey(2, "E")}
	if reflect.DeepEqual(load.ItemKeys, test) != true {
		t.Errorf("Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.ItemKeys, test)
	}
	test = ItemKeyList{genItemKey(10, "IA"), genItemKey(8, "IB"), genItemKey(2, "IE")}
	if reflect.DeepEqual(load.InsertedItemKeys, test) != true {
		t.Errorf("Inserted Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.InsertedItemKeys, test)
	}
}

func TestFeedItemsResolvingPreDeletedItems(t *testing.T) { // Only ensures the lists turn out what I want.
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Entry := Feed{
		Url: *testFeedUrl,

		Title:     "First Title",
		LastCheck: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),

		NextCheck: time.Date(2013, 7, 1, 1, 0, 0, 0, time.UTC),

		ItemKeys:        ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "A"), genItemKey(8, "B")},
		DeletedItemKeys: ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")},
		//Inserted item keys left untested here, as they are tested below
	}

	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Using same base data, add more keys.  Pretend A fell off a cliff, and add an E.  DeletedItems clears.
	Entry.ItemKeys = ItemKeyList{genItemKey(100, "AA"), genItemKey(40, "CC"), genItemKey(8, "B"), genItemKey(2, "E")}
	Entry.DeletedItemKeys = ItemKeyList{}

	// Clear the model to make a sibling.
	Entry.Model = riak.Model{}
	//And save
	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Cause conflict
	load := Feed{}
	if err := con.LoadModel("ConflictFeed", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	}

	// Verify lists.
	test := ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")}
	if reflect.DeepEqual(load.DeletedItemKeys, test) != true {
		t.Errorf("Deleted Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.DeletedItemKeys, ItemKeyList{genItemKey(40, "C"), genItemKey(30, "D")})
	}
	test = ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "A"), genItemKey(8, "B"), genItemKey(2, "E")}
	if reflect.DeepEqual(load.ItemKeys, test) != true {
		t.Errorf("Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.ItemKeys, test)
	}
}

func TestFeedItemsResolvingPreDealtWithInsertedItems(t *testing.T) { // Only ensures the lists turn out what I want.
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Entry := Feed{
		Url: *testFeedUrl,

		Title:     "First Title",
		LastCheck: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),

		NextCheck: time.Date(2013, 7, 1, 1, 0, 0, 0, time.UTC),

		ItemKeys:         ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "A"), genItemKey(8, "B")},
		DeletedItemKeys:  ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")},
		InsertedItemKeys: ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "IA"), genItemKey(8, "IB")},
	}

	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Using same base data, add more keys.  Pretend A fell off a cliff, and add an E.  DeletedItems clears.
	Entry.ItemKeys = ItemKeyList{genItemKey(100, "AA"), genItemKey(8, "B"), genItemKey(2, "E")}
	Entry.InsertedItemKeys = ItemKeyList{genItemKey(40, "CC"), genItemKey(8, "IB"), genItemKey(2, "IE")}
	Entry.DeletedItemKeys = ItemKeyList{}

	// Clear the model to make a sibling.
	Entry.Model = riak.Model{}
	//And save
	if err := con.NewModel("ConflictFeed", &Entry); err != nil {
		t.Fatalf("Failed to create Entry's model (%s)", err)
	}
	if err := Entry.Save(); err != nil {
		t.Fatalf("Failed to save Entry (%s)", err)
	}

	// Cause conflict
	load := Feed{}
	if err := con.LoadModel("ConflictFeed", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	}

	// Verify lists.
	test := ItemKeyList{genItemKey(40, "CC"), genItemKey(30, "DD")}
	if reflect.DeepEqual(load.DeletedItemKeys, test) != true {
		t.Errorf("Deleted Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.DeletedItemKeys, ItemKeyList{genItemKey(40, "C"), genItemKey(30, "D")})
	}
	test = ItemKeyList{genItemKey(100, "AA"), genItemKey(10, "A"), genItemKey(8, "B"), genItemKey(2, "E")}
	if reflect.DeepEqual(load.ItemKeys, test) != true {
		t.Errorf("Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.ItemKeys, test)
	}
	test = ItemKeyList{genItemKey(10, "IA"), genItemKey(8, "IB"), genItemKey(2, "IE")}
	if reflect.DeepEqual(load.InsertedItemKeys, test) != true {
		t.Errorf("Inserted Item Keys didn't match as expected!  Returned: %v, Wanted: %v", load.InsertedItemKeys, test)
	}
}

func TestFeedItemResolving(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	url, _ := url.Parse("http://example.com/story_up_1")
	Item := FeedItem{
		Url: *url,

		Title:   "First Title",
		Author:  "Author 1",
		Content: "Content 1",
		PubDate: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := con.NewModel("ConflictItem", &Item); err != nil {
		t.Fatalf("Failed to create Item's model (%s)", err)
	}
	if err := Item.Save(); err != nil {
		t.Fatalf("Failed to save Item (%s)", err)
	}

	// And totally new actually used data from an update.
	url, _ = url.Parse("http://example.com/story_up_2")
	Item = FeedItem{
		Url: *url,

		Title:   "Second Title",
		Author:  "Author 2",
		Content: "Content 2",
		PubDate: time.Date(2014, 7, 1, 0, 0, 0, 0, time.UTC),
	}

	// Clear the model to make a sibling.
	Item.Model = riak.Model{}
	//And save
	if err := con.NewModel("ConflictItem", &Item); err != nil {
		t.Fatalf("Failed to create Item's model (%s)", err)
	}
	if err := Item.Save(); err != nil {
		t.Fatalf("Failed to save Item (%s)", err)
	}

	// Cause conflict and verify resolve function
	load := FeedItem{}
	if err := con.LoadModel("ConflictItem", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	} else if compareItems(Item, load) == false {
		t.Errorf("Resolved model does not match latest update (old, new) (%v, %v)", Item, load)
	}
}
