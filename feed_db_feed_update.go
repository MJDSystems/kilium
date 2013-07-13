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

	"sort"

	riak "github.com/tpjg/goriakpbc"
)

var FeedNotFound = errors.New("Failed to find feed in riak!")

const (
	MaximumFeedItems = 10000
)

func drainErrorChannelIntoSlice(errCh <-chan error, errorSlice *[]error, responses int) {
	for i := 0; i < responses; i++ {
		err := <-errCh
		if err != nil {
			*errorSlice = append(*errorSlice, err)
		}
	}
}

func InsertItem(con *riak.Client, itemKey ItemKey, item ParsedFeedItem) error {
	itemModel := FeedItem{
		Title:   item.Title,
		Author:  item.Author,
		Content: item.Content,
		Url:     item.Url,
		PubDate: item.PubDate,
	}
	if err := con.NewModel(string(itemKey), &itemModel); err != nil {
		return err
	} else if err = itemModel.Save(); err != nil {
		return err
	}
	return nil
}

func UpdateItem(con *riak.Client, itemKey ItemKey, item ParsedFeedItem, itemModel *FeedItem) error {
	itemModel.Title = item.Title
	itemModel.Author = item.Author
	itemModel.Content = item.Content
	itemModel.Url = item.Url
	itemModel.PubDate = item.PubDate

	if err := itemModel.Save(); err != nil {
		return err
	}
	return nil
}

func itemDiffersFromModel(feedItem ParsedFeedItem, model *FeedItem) bool {
	return true
}

func updateFeed(con *riak.Client, feedUrl url.URL, feedData ParsedFeedData, ids <-chan uint64) error {
	feed := &Feed{Url: feedUrl}
	if err := con.LoadModel(feed.UrlKey(), feed); err == riak.NotFound {
		return FeedNotFound
	} else if err != nil {
		return err
	}
	// First finish clean off all deleted items, and clean out inserted item keys.  Since either existing
	// means a previous operation has been left in a bad state.  TBI

	// Next update the basic attributes (title basically)
	feed.Title = feedData.Title

	/* Next find all the feed items to insert/update.  If the item doesn't exist, create it's id and
	 * mark for insert.  Otherwise mark it for an read/update/store pass.  Make sure to mark for
	 * deletion items as necessary.
	 */
	// This struct holds an ItemKey and a ParsedFeedItem for later parsing.
	type ToProcess struct {
		ItemKey ItemKey
		Data    ParsedFeedItem
		Model   *FeedItem
	}
	NewItems := make([]ToProcess, 0)
	UpdatedItems := make([]ToProcess, 0)

	for _, rawItem := range feedData.Items {
		// Try to find the raw Item in the Item Keys list.
		index := feed.ItemKeys.FindRawItemId(rawItem.GenericKey)
		if index != -1 {
			// Found it!  Load the details.  Also load the model, which will be re-used later.
			p := ToProcess{
				ItemKey: feed.ItemKeys[index],
				Data:    rawItem,
				Model:   &FeedItem{},
			}

			if err := con.LoadModel(string(p.ItemKey), p.Model); err != nil {
				return err
			}

			// Ok, now is this have a new pub date?  If so, pull it out of its current position, and
			// move it up the chain.
			if p.Model.PubDate.Equal(p.Data.PubDate) && !(p.Data.PubDate.IsZero() && itemDiffersFromModel(p.Data, p.Model)) {
				// Pub dates are the same.  Just modify the item to match what is in the feed.
				UpdatedItems = append(UpdatedItems, p)
			} else {
				// Pub dates differ.  Delete the item, and re-insert it.
				feed.DeletedItemKeys = append(feed.DeletedItemKeys, p.ItemKey)
				feed.ItemKeys.RemoveAt(index)

				// Delete the model from the to process struct.
				p.Model = &FeedItem{}

				NewItems = append(NewItems, p) // This gives us the new id.
			}
		} else {
			// Nope, lets insert it!
			NewItems = append(NewItems, ToProcess{
				Data: rawItem,
			})
		}
	}

	/* Alright, any new items are mentioned in the Feed before being inserted.  In case something
	 * happens, I'd prefer not to lose an item.  Note the order is reversed so that the oldest story
	 * will get the smallest id, preserving sort order.  Inserted Item Keys needs to be sorted (well,
	 * reversed) after this so it is in correct order as well.  This loop violates ItemKeys sort
	 * order, so the sort is necessary for now. */
	for i := len(NewItems) - 1; i >= 0; i-- {
		newItem := &NewItems[i]
		newItem.ItemKey = NewItemKey(<-ids, newItem.Data.GenericKey)
		feed.InsertedItemKeys = append(feed.InsertedItemKeys, newItem.ItemKey)
	}
	sort.Sort(feed.InsertedItemKeys)

	// Ok, we must save here.  Otherwise planned changes may occur that will not be cleaned up!
	if err := feed.Save(); err != nil {
		return err
	}

	errCh := make(chan error) // All of the errors go into here, to be pulled out.

	// Good, now implement the change and update the Feed.

	// First add new items
	for _, newItem := range NewItems {
		feed.ItemKeys = append(feed.ItemKeys, newItem.ItemKey)
		go func(newItem ToProcess) {
			errCh <- InsertItem(con, newItem.ItemKey, newItem.Data)
		}(newItem)
	}
	feed.InsertedItemKeys = nil

	// Now update them.
	for _, newItem := range UpdatedItems {
		go func(newItem ToProcess) {
			errCh <- UpdateItem(con, newItem.ItemKey, newItem.Data, newItem.Model)
		}(newItem)
	}

	// Finally delete items.
	for _, deleteItemKey := range feed.DeletedItemKeys {
		go func(toDelete ItemKey) {
			errCh <- con.DeleteFrom("items", string(toDelete))
		}(deleteItemKey)
	}
	deletedItemCount := len(feed.DeletedItemKeys) // Need this to drain the error channel later.
	// Ok, deleted.  So clear the list
	feed.DeletedItemKeys = nil

	sort.Sort(sort.Reverse(feed.ItemKeys)) // Just sort this.  TBD: Actually maintain this sort order to avoid this!

	//Now, collect the errors
	var errs []error
	drainErrorChannelIntoSlice(errCh, &errs, len(NewItems))
	drainErrorChannelIntoSlice(errCh, &errs, len(UpdatedItems))
	drainErrorChannelIntoSlice(errCh, &errs, deletedItemCount)
	if len(errs) != 0 {
		return MultiError(errs)
	}

	if err := feed.Save(); err != nil {
		return err
	}

	_, _ = NewItems, UpdatedItems
	return nil
}
