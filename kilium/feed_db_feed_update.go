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
package kilium

import (
	"errors"

	"net/url"

	"sort"
	"strconv"

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
	if err := con.LoadModel(itemKey.GetRiakKey(), &itemModel); err != riak.NotFound {
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

func itemDiffersFromModel(feedItem ParsedFeedItem, itemModel *FeedItem) bool {
	return itemModel.Title != feedItem.Title ||
		itemModel.Author != feedItem.Author ||
		itemModel.Content != feedItem.Content ||
		itemModel.Url != feedItem.Url ||
		itemModel.PubDate != feedItem.PubDate
}

func updateFeed(con *riak.Client, feedUrl url.URL, feedData ParsedFeedData, ids <-chan uint64) (*Feed, error) {
	feed := &Feed{Url: feedUrl}
	if err := con.LoadModel(feed.UrlKey(), feed); err == riak.NotFound {
		return nil, FeedNotFound
	} else if err != nil {
		return nil, err
	}
	// First clean out inserted item keys.  This handles unfinished previous operations.
	itemsBucket, err := con.Bucket("items")
	if err != nil {
		return nil, err
	}

	// Note, this insert items without caring about the 10,000 limit.  Of course, any regular inserted
	// item will force the limit back down.
	for _, itemKey := range feed.InsertedItemKeys {
		// Does this item exist?
		if ok, err := itemsBucket.Exists(itemKey.GetRiakKey()); err != nil {
			return nil, err
		} else if ok {
			// Yep, so add it to the list.
			feed.ItemKeys = append(feed.ItemKeys, itemKey)
		}
		// Otherwise non-existent items are dropped.  This is to avoid
	}
	feed.InsertedItemKeys = nil

	// Next update the basic attributes
	feed.Title = feedData.Title
	feed.NextCheck = feedData.NextCheckTime
	feed.LastCheck = feedData.FetchedAt
	// Also set 2i to appropriate values!
	feed.Indexes()[NextCheckIndexName] = strconv.FormatInt(feed.NextCheck.Unix(), 10)

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
	SeenNewItemKeys := make(map[string]bool)

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

			if err := con.LoadModel(p.ItemKey.GetRiakKey(), p.Model); err != nil {
				return nil, err
			}

			// Ok, now is this have a new pub date?  If so, pull it out of its current position, and
			// move it up the chain.  Otherwise, just update the content.  If an item has no pub date,
			// assume that it has changed if the any part of the item changed.
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
			// Nope, lets insert it!  First, should we knock off an item?  e need to stay below MaximumFeedItems.
			for (len(feed.ItemKeys)+len(NewItems)) >= MaximumFeedItems && len(feed.ItemKeys) > 0 {
				// Need to kill an item.  So get the last key
				lastKey := feed.ItemKeys[len(feed.ItemKeys)-1]
				// insert it onto the end of the deleted item list.
				feed.DeletedItemKeys = append(feed.DeletedItemKeys, lastKey)
				// If we are updating this key, then remove it from this list.  No need to waste
				// time.
				for i, item := range UpdatedItems {
					if item.ItemKey.Equal(lastKey) {
						UpdatedItems = append(UpdatedItems[:i], UpdatedItems[i+1:]...)
					}
				}
				// And finally, pop the item
				feed.ItemKeys = feed.ItemKeys[:len(feed.ItemKeys)-1]
			}
			// Only insert if there are less then MaximumFeedItems already to be inserted.
			// This works since any later item will have been updated after.
			if len(NewItems) < MaximumFeedItems {
				// Also, make sure we aren't inserting the same item twice.  If it is duplicated, the
				// second item is guaranteed to be later.  So just drop it.
				if keyString := string(rawItem.GenericKey); SeenNewItemKeys[keyString] == false {
					NewItems = append(NewItems, ToProcess{
						Data: rawItem,
					})
					SeenNewItemKeys[keyString] = true
				}
			}
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
		return nil, err
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
			if obj, err := itemsBucket.Get(toDelete.GetRiakKey()); obj == nil {
				errCh <- err
			} else {
				errCh <- obj.Destroy()
			}
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
		return nil, MultiError(errs)
	}

	if err := feed.Save(); err != nil {
		return nil, err
	}

	return feed, nil
}

type UpdatedModel struct {
	Url   url.URL
	Model *Feed
}

func UpdateFeed(con *riak.Client, idGenerator <-chan uint64, in <-chan FeedParserOut, out chan<- UpdatedModel, errChan chan<- FeedError) {
	for {
		if next, ok := <-in; ok {
			model, err := updateFeed(con, next.Url, next.Data, idGenerator)
			if err != nil {
				errChan <- FeedError{err, next.Url}
			} else {
				out <- UpdatedModel{next.Url, model}
			}
		} else {
			break
		}
	}
}
