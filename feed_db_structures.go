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

	"encoding/base64"

	"net/url"
	"time"

	riak "github.com/tpjg/goriakpbc"
)

type ItemKey []byte

func (l ItemKey) Less(r Comparable) bool {
	return bytes.Compare(l, r.(ItemKey)) == -1
}
func (l *ItemKey) UnmarshalJSON(input []byte) error {
	input = input[1 : len(input)-1]

	*l = make(ItemKey, base64.StdEncoding.DecodedLen(len(input)))
	n, err := base64.StdEncoding.Decode(*l, input)

	if err != nil {
		return err
	} else {
		*l = (*l)[0:n]
		return nil
	}
}

type ItemKeyList []ItemKey

func (list *ItemKeyList) Append(key Comparable) {
	*list = append(*list, key.(ItemKey))
}

func (list ItemKeyList) Get(index int) Comparable {
	return list[index]
}

func (list ItemKeyList) Len() int {
	return len(list)
}

func (ItemKeyList) Make() ComparableArray {
	return &ItemKeyList{}
}

type Feed struct {
	Url url.URL `riak:"url"`

	Title     string    `riak:"title"`
	LastCheck time.Time `riak:"last_check"`

	ItemKeys        ItemKeyList `riak:"item_keys"`
	DeletedItemKeys ItemKeyList `riak:"deleted_items"`

	NextCheck time.Time `riak:"next_check"`

	riak.Model `riak:"feeds"`
}

func (f *Feed) Resolve(siblingsCount int) error {
	// First get the siblings!
	siblingsI, err := f.Siblings(&Feed{})
	if err != nil {
		return err
	}
	siblings := siblingsI.([]Feed)

	// Next, just use the first sibling as the default values.  Everything merges against it.
	*f = siblings[0]

	for i := 1; i < siblingsCount; i++ {
		// Resolve regular feed details.  Basically, take the latest version!
		if siblings[i].LastCheck.After(f.LastCheck) {
			f.Title = siblings[i].Title
			f.LastCheck = siblings[i].LastCheck
			f.NextCheck = siblings[i].NextCheck
		}

		// for the item lists, merge and de-dup using insert slice sort!
		f.ItemKeys = *InsertSliceSort(&f.ItemKeys, &siblings[i].ItemKeys).(*ItemKeyList)
		f.DeletedItemKeys = *InsertSliceSort(&f.DeletedItemKeys, &siblings[i].DeletedItemKeys).(*ItemKeyList)
	}
	return nil
}

type FeedItem struct {
	riak.Model
}
